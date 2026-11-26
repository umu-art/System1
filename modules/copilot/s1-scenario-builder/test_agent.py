import json
import unittest

from agent import Agent, Specification, VALIDATION_INSTRUCTIONS


def specification(trigger="/voice"):
    return {
        "prd": {
            "title": "Озвучка",
            "audience": "Клиент",
            "outcome": "Получить аудио",
            "requirements": [{"id": "r1", "text": "Озвучить текст", "source": "owner"}],
        },
        "workflows": [{
            "title": "Озвучка", "trigger": trigger, "outcome": "Аудио выслано", "requirement_ids": ["r1"],
        }],
    }


def tool(name, **args):
    return [{
        "type": "function_call", "name": name, "call_id": f"call_{name}", "arguments": json.dumps(args),
    }]


def message(text):
    return [{"type": "message", "content": [{"type": "output_text", "text": text}]}]


class Model:
    def __init__(self, *responses):
        self.responses = list(responses)
        self.validations = []

    def __call__(self, messages, instructions, **options):
        if instructions == VALIDATION_INSTRUCTIONS:
            self.validations.append(json.loads(messages[0]["content"]))
        return self.responses.pop(0)


class AgentTest(unittest.TestCase):
    def test_interview_and_revision_after_clarification(self):
        model = Model(
            tool("ask_owner", question="Что должен получить клиент?"),
            tool("save_specification", **specification()),
            tool("validate_specification"),
            message('{"specification_issues":[],"blocking_questions":["Как клиент запускает озвучку?"]}'),
            tool("ask_owner", question="Как клиент запускает озвучку?"),
            tool("save_specification", **specification("/audio")),
            tool("validate_specification"),
            message('{"specification_issues":[],"blocking_questions":[]}'),
            message("Спецификация готова к согласованию."),
        )
        agent = Agent(model)
        self.assertEqual(agent.turn("Хочу бота."), "Что должен получить клиент?")
        self.assertEqual(agent.revision, 0)
        self.assertIsNone(agent.specification)
        self.assertEqual(agent.turn("Клиент отправляет текст и получает аудио."), "Как клиент запускает озвучку?")
        self.assertFalse(agent.ready)
        self.assertEqual(agent.revision, 1)
        self.assertEqual(agent.turn("Командой /audio."), "Спецификация готова к согласованию.")
        self.assertTrue(agent.ready)
        self.assertEqual(agent.revision, 2)
        self.assertEqual(agent.specification.workflows[0].trigger, "/audio")
        self.assertEqual(model.validations[-1]["dialog"][-2]["content"], "Как клиент запускает озвучку?")
        self.assertEqual(model.validations[-1]["dialog"][-1]["content"], "Командой /audio.")
        self.assertFalse(model.responses)

    def test_previous_validation_does_not_approve_new_revision(self):
        clean = message('{"specification_issues":[],"blocking_questions":[]}')
        model = Model(clean, clean)
        agent = Agent(model)
        agent.save_specification(Specification.model_validate(specification()))
        self.assertEqual(agent.validate_specification()["revision"], 1)
        self.assertTrue(agent.ready)
        agent.save_specification(Specification.model_validate(specification("/audio")))
        self.assertFalse(agent.ready)
        self.assertEqual(agent.validate_specification()["revision"], 2)
        self.assertEqual(model.validations[-1]["specification"]["workflows"][0]["trigger"], "/audio")

    def test_model_cannot_supply_a_revision(self):
        agent = Agent(Model(tool("validate_specification", revision=1), message("Invalid arguments.")))
        agent.turn("Validate the specification.")
        result = next(item for item in agent.history if item.get("type") == "function_call_output")
        self.assertEqual(json.loads(result["output"]), {"error": "Arguments do not match the tool schema"})

    def test_unknown_requirement_reaches_model_as_tool_error(self):
        invalid = specification()
        invalid["workflows"][0]["requirement_ids"] = ["missing"]
        model = Model(tool("save_specification", **invalid), tool("ask_owner", question="Уточните требование."))
        agent = Agent(model)
        agent.turn("Опишите сценарий.")
        response = next(item for item in agent.history if item.get("type") == "function_call_output")
        self.assertIn("unknown requirement", json.loads(response["output"])["error"])
        self.assertIsNone(agent.specification)

    def test_validation_can_block_without_asking_the_owner(self):
        model = Model(
            tool("save_specification", **specification()),
            tool("validate_specification"),
            message('{"specification_issues":["The /audio command was lost."],"blocking_questions":[]}'),
            message("The specification needs a correction."),
        )
        agent = Agent(model)
        agent.turn("The entry command is /audio.")
        self.assertFalse(agent.ready)
        result = next(
            item for item in agent.history
            if item.get("type") == "function_call_output" and item.get("call_id") == "call_validate_specification"
        )
        self.assertEqual(json.loads(result["output"])["specification_issues"], ["The /audio command was lost."])


if __name__ == "__main__":
    unittest.main()
