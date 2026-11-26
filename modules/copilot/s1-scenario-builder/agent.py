import argparse
import json
import os
from contextlib import contextmanager
from pathlib import Path
from typing import Annotated, Literal

import httpx
from dotenv import load_dotenv
from pydantic import BaseModel, ConfigDict, Field, StringConstraints, ValidationError


PROMPTS_DIR = Path(__file__).with_name("prompts")
AGENT_INSTRUCTIONS = (PROMPTS_DIR / "scenario_agent.md").read_text(encoding="utf-8")
VALIDATION_INSTRUCTIONS = (PROMPTS_DIR / "scenario_validation.md").read_text(encoding="utf-8")


class Record(BaseModel):
    model_config = ConfigDict(extra="forbid")


Text = Annotated[str, StringConstraints(strip_whitespace=True, min_length=1)]


class Question(Record):
    question: Text


class Requirement(Record):
    id: Text
    text: Text
    source: Literal["owner", "proposal"]


class PRD(Record):
    title: Text
    audience: Text
    outcome: Text
    requirements: list[Requirement] = Field(min_length=1)


class Workflow(Record):
    title: Text
    trigger: Text
    outcome: Text
    requirement_ids: list[Text] = Field(min_length=1)


class Specification(Record):
    prd: PRD
    workflows: list[Workflow] = Field(min_length=1)


class ValidationRequest(Record):
    pass


class Validation(Record):
    specification_issues: list[Text]
    blocking_questions: list[Text]


TOOLS = {
    "ask_owner": (Question, "Show one question to the owner and wait for their answer."),
    "save_specification": (Specification, "Save the complete PRD and workflow map and return its revision."),
    "validate_specification": (ValidationRequest, "Validate the current saved specification against the dialogue. Takes no arguments. Return specification issues and questions for the owner."),
}


def tool_schemas():
    return [
        {"type": "function", "name": name, "description": description, "strict": True,
         "parameters": model.model_json_schema()}
        for name, (model, description) in TOOLS.items()
    ]


def output_text(output):
    return "\n".join(
        part["text"] for item in output if item["type"] == "message"
        for part in item["content"] if part["type"] == "output_text"
    )


class TimewebModerationError(RuntimeError):
    pass


class Timeweb:
    def __init__(self, client, model):
        self.client = client
        self.model = model

    def respond(self, messages, instructions, **options):
        try:
            result = self.client.post("responses", json={
                "model": self.model,
                "input": messages,
                "instructions": instructions,
                "reasoning": {"effort": "none"},
                "store": False,
                "max_output_tokens": 4000,
                **options,
            })
            result.raise_for_status()
            payload = result.json()
        except httpx.HTTPStatusError as error:
            response = error.response
            details = None
            if response.status_code == 400:
                try:
                    payload = response.json()
                    details = payload.get("error") if isinstance(payload, dict) else None
                except ValueError:
                    pass
            if isinstance(details, dict) and "flagged by our content moderation system" in str(details.get("message", "")).lower():
                raise TimewebModerationError("Timeweb flagged the message for moderation. Rephrase it and try again.") from None
            raise RuntimeError(f"Timeweb HTTP {response.status_code}") from None
        except (httpx.RequestError, ValueError):
            raise RuntimeError("Timeweb request failed") from None
        if payload.get("status") != "completed":
            raise RuntimeError("Timeweb response incomplete")
        return payload["output"]


class Agent:
    def __init__(self, respond):
        self.respond = respond
        self.history = []
        self.dialog = []
        self.specification = None
        self.revision = 0
        self.validation = None

    @property
    def ready(self):
        return self.validation is not None and not (self.validation.specification_issues or self.validation.blocking_questions)

    def turn(self, text):
        message = {"role": "user", "content": text}
        self.history.append(message)
        self.dialog.append(message)
        for _ in range(10):
            output = self.respond(
                self.history, AGENT_INSTRUCTIONS, tools=tool_schemas(), parallel_tool_calls=False,
            )
            self.history.extend(output)
            call = next((item for item in output if item["type"] == "function_call"), None)
            if call is None:
                reply = output_text(output)
                if reply:
                    self.dialog.append({"role": "assistant", "content": reply})
                return reply
            question = self.execute(call)
            if question:
                self.dialog.append({"role": "assistant", "content": question})
                return question
        raise RuntimeError("Tool call limit reached (10 per turn)")

    def execute(self, call):
        name = call["name"]
        result = {}
        question = None
        try:
            arguments = TOOLS[name][0].model_validate_json(call["arguments"])
            if name == "ask_owner":
                question = arguments.question
            elif name == "save_specification":
                result = self.save_specification(arguments)
            elif name == "validate_specification":
                result = self.validate_specification()
        except KeyError:
            result = {"error": "Unknown tool"}
        except ValidationError:
            result = {"error": "Arguments do not match the tool schema"}
        except TimewebModerationError:
            raise
        except (ValueError, RuntimeError) as error:
            result = {"error": str(error)}

        self.history.append({
            "type": "function_call_output", "call_id": call["call_id"],
            "output": json.dumps(result, ensure_ascii=False),
        })
        return question

    def save_specification(self, specification):
        ids = [item.id for item in specification.prd.requirements]
        if len(set(ids)) != len(ids):
            raise ValueError("Requirement IDs must be unique")
        for item in specification.workflows:
            if not set(item.requirement_ids).issubset(ids):
                raise ValueError(f"Workflow {item.title!r} references an unknown requirement")
        self.specification = specification
        self.validation = None
        self.revision += 1
        return {"revision": self.revision}

    def validate_specification(self):
        if self.specification is None:
            raise ValueError("Save a specification before validating it")
        if self.validation is None:
            context = {"dialog": self.dialog, "specification": self.specification.model_dump()}
            output = self.respond(
                [{"role": "user", "content": json.dumps(context, ensure_ascii=False)}],
                VALIDATION_INSTRUCTIONS,
                text={"format": {"type": "json_schema", "name": "validation", "strict": True,
                                 "schema": Validation.model_json_schema()}},
            )
            self.validation = Validation.model_validate_json(output_text(output))
        return {"revision": self.revision, **self.validation.model_dump()}


@contextmanager
def timeweb_session():
    key = os.getenv("OPENAI_API_KEY")
    if not key:
        raise RuntimeError("OPENAI_API_KEY не задан.")
    model = os.getenv("TIMEWEB_MODEL", "openai/gpt-6-luna")
    base_url = os.getenv("TIMEWEB_BASE_URL", "https://api.timeweb.ai/v1").rstrip("/")
    with httpx.Client(
        base_url=base_url + "/", headers={"Authorization": f"Bearer {key}"}, timeout=60,
    ) as client:
        yield Timeweb(client, model)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--trace", action="store_true", help="Show tool calls")
    args = parser.parse_args()
    load_dotenv(Path(__file__).with_name(".env"))
    with timeweb_session() as provider:
        agent = Agent(provider.respond)
        print("System1 · /exit")
        try:
            while True:
                text = input("> ").strip()
                if text == "/exit":
                    break
                if not text:
                    continue
                start = len(agent.history)
                revision, was_ready = agent.revision, agent.ready
                reply = agent.turn(text)
                if args.trace:
                    for item in agent.history[start:]:
                        if item.get("type") == "function_call":
                            print(f"[{item['name']}]")
                print(reply)
                if agent.revision != revision or (agent.ready and not was_ready):
                    print(f"Specification #{agent.revision}")
                    print(agent.specification.model_dump_json(indent=2))
        except (EOFError, KeyboardInterrupt):
            return


if __name__ == "__main__":
    try:
        main()
    except RuntimeError as error:
        raise SystemExit(str(error)) from None
