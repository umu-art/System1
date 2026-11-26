import json
from pathlib import Path
from threading import Lock
from uuid import UUID, uuid4

from dotenv import load_dotenv
from fastapi import APIRouter, HTTPException
from fastapi.responses import FileResponse
from pydantic import BaseModel

from agent import Agent, Text, TimewebModerationError, timeweb_session

router = APIRouter()
root = Path(__file__).resolve().parent
load_dotenv(root / ".env")

TRACE_TOOLS = {"ask_owner", "save_specification", "validate_specification"}


def pipeline_trace(history):
    trace = []
    pending = {}
    turn = 0
    for item in history:
        if item.get("role") == "user":
            turn += 1
        elif item.get("type") == "function_call" and item["name"] in TRACE_TOOLS:
            try:
                arguments = json.loads(item["arguments"])
            except (ValueError, TypeError):
                arguments = item.get("arguments")
            event = {"turn": turn, "tool": item["name"], "arguments": arguments, "result": None}
            trace.append(event)
            pending[item["call_id"]] = event
        elif item.get("type") == "function_call_output":
            event = pending.pop(item["call_id"], None)
            if event is not None:
                event["result"] = json.loads(item["output"])
    return trace


class Session:
    def __init__(self):
        self.id = uuid4()
        self.agent = Agent(respond)
        self.lock = Lock()

    def snapshot(self):
        return {
            "id": self.id,
            "dialog": list(self.agent.dialog),
            "specification": self.agent.specification,
            "revision": self.agent.revision,
            "validation": self.agent.validation,
            "trace": pipeline_trace(self.agent.history),
        }


sessions: dict[UUID, Session] = {}


def respond(*args, **kwargs):
    with timeweb_session() as provider:
        return provider.respond(*args, **kwargs)


class Message(BaseModel):
    text: Text


@router.get("/")
def home():
    return FileResponse(root / "beta.html")


@router.post("/beta/sessions")
def create_session():
    session = Session()
    sessions[session.id] = session
    return session.snapshot()


@router.get("/beta/sessions/{session_id}")
def get_session(session_id: UUID):
    session = sessions.get(session_id)
    if session is None:
        raise HTTPException(404, "Диалог не найден.")
    with session.lock:
        return session.snapshot()


@router.post("/beta/sessions/{session_id}/messages")
def send_message(session_id: UUID, message: Message):
    session = sessions.get(session_id)
    if session is None:
        raise HTTPException(404, "Диалог не найден.")
    if not session.lock.acquire(blocking=False):
        raise HTTPException(409, "Предыдущее сообщение ещё обрабатывается.")
    try:
        agent = session.agent
        history_size, dialog_size = len(agent.history), len(agent.dialog)
        specification, validation, revision = agent.specification, agent.validation, agent.revision
        try:
            agent.turn(message.text)
        except RuntimeError as error:
            del agent.history[history_size:]
            del agent.dialog[dialog_size:]
            agent.specification, agent.validation, agent.revision = specification, validation, revision
            if isinstance(error, TimewebModerationError):
                raise HTTPException(422, "Timeweb отклонил реплику при проверке модерацией. Переформулируй сообщение и отправь снова.") from error
            raise HTTPException(502, str(error)) from error
        return session.snapshot()
    finally:
        session.lock.release()
