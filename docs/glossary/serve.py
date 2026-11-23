#!/usr/bin/env python3
"""Local editor for the System1 glossary.

Run:  python3 docs/glossary/serve.py [--port 8000]
Then open http://127.0.0.1:8000 and edit terms in the page.
Each term is stored as docs/glossary/terms/<id>.json; sections live in meta.json.
The id follows the English name (service_spec -> service-spec.json): when the page
changes a term's English name, it sends "_from": <old id> and the file is renamed.
Commit them as usual.

Only the Python standard library is used. The server listens on 127.0.0.1 only.
"""

import argparse
import json
import re
import threading
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

ROOT = Path(__file__).resolve().parent
TERMS = ROOT / "terms"
META = ROOT / "meta.json"
INDEX = ROOT / "index.html"

ID_RE = re.compile(r"^[a-z0-9][a-z0-9-]{0,63}$")
KEY_ORDER = ["group", "order", "owner", "status", "ru", "en", "zh", "def", "ex", "conflicts"]
MAX_BODY = 256 * 1024
WRITE_LOCK = threading.Lock()


def canonical(term: dict) -> dict:
    """Fixed key order and shape, so diffs stay small and predictable."""
    def bi(v):
        v = v if isinstance(v, dict) else {}
        return {"zh": str(v.get("zh", "")), "ru": str(v.get("ru", ""))}

    out = {
        "group": str(term.get("group", "")),
        "order": int(term.get("order", 0) or 0),
        "owner": str(term.get("owner", "none")),
        "status": str(term.get("status", "prop")),
        "ru": str(term.get("ru", "")),
        "en": str(term.get("en", "")),
        "zh": str(term.get("zh", "")),
        "def": bi(term.get("def")),
        "ex": bi(term.get("ex")),
        "conflicts": [str(c) for c in term.get("conflicts", []) if str(c).strip()],
    }
    return {k: out[k] for k in KEY_ORDER}


def canonical_groups(groups) -> list:
    if not isinstance(groups, list):
        raise ValueError("groups must be a list")
    out, seen = [], set()
    for g in groups:
        if not isinstance(g, dict):
            raise ValueError("each group must be an object")
        gid = str(g.get("id", ""))
        if not ID_RE.match(gid):
            raise ValueError(f"bad group id {gid!r} (use a-z, 0-9, -)")
        if gid in seen:
            raise ValueError(f"duplicate group id {gid!r}")
        seen.add(gid)
        out.append({"id": gid, "zh": str(g.get("zh", "")), "ru": str(g.get("ru", ""))})
    return out


def term_counts_by_group() -> dict:
    counts = {}
    for f in TERMS.glob("*.json"):
        try:
            g = json.loads(f.read_text(encoding="utf-8")).get("group")
        except (json.JSONDecodeError, AttributeError):
            continue
        counts[g] = counts.get(g, 0) + 1
    return counts


def dump(obj) -> str:
    return json.dumps(obj, ensure_ascii=False, indent=2) + "\n"


def write_atomic(path: Path, obj) -> None:
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_text(dump(obj), encoding="utf-8")
    tmp.replace(path)


def load_all() -> dict:
    meta = json.loads(META.read_text(encoding="utf-8"))
    terms = []
    for f in sorted(TERMS.glob("*.json")):
        try:
            data = json.loads(f.read_text(encoding="utf-8"))
        except json.JSONDecodeError as e:
            terms.append({"id": f.stem, "_error": f"{f.name}: {e}"})
            continue
        terms.append({"id": f.stem, **data})
    return {"meta": meta, "terms": terms}


class Handler(BaseHTTPRequestHandler):
    server_version = "glossary/1"

    def log_message(self, fmt, *args):  # quieter console
        if not self.path.startswith("/api/glossary"):
            super().log_message(fmt, *args)

    def send_json(self, status, obj):
        body = json.dumps(obj, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(body)

    def term_path(self):
        m = re.fullmatch(r"/api/terms/([^/]+)", self.path)
        if not m or not ID_RE.match(m.group(1)):
            self.send_json(HTTPStatus.BAD_REQUEST, {"error": "bad term id (use a-z, 0-9, -)"})
            return None
        return TERMS / f"{m.group(1)}.json"

    def do_GET(self):
        if self.path in ("/", "/index.html"):
            body = INDEX.read_bytes()
            self.send_response(HTTPStatus.OK)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.send_header("Cache-Control", "no-store")
            self.end_headers()
            self.wfile.write(body)
        elif self.path == "/api/glossary":
            self.send_json(HTTPStatus.OK, load_all())
        else:
            self.send_json(HTTPStatus.NOT_FOUND, {"error": "not found"})

    def read_object(self):
        length = int(self.headers.get("Content-Length", 0))
        if length <= 0 or length > MAX_BODY:
            raise ValueError("empty or too large body")
        obj = json.loads(self.rfile.read(length).decode("utf-8"))
        if not isinstance(obj, dict):
            raise ValueError("body must be a JSON object")
        return obj

    def do_PUT(self):
        if self.path == "/api/groups":
            self.put_groups()
            return
        path = self.term_path()
        if path is None:
            return
        try:
            obj = self.read_object()
            src = obj.pop("_from", None)  # old id when the English name (and so the id) changed
            data = canonical(obj)
            if src is not None and not ID_RE.match(str(src)):
                raise ValueError("bad _from id (use a-z, 0-9, -)")
        except (ValueError, TypeError) as e:
            self.send_json(HTTPStatus.BAD_REQUEST, {"error": str(e)})
            return
        old = TERMS / f"{src}.json" if src is not None and src != path.stem else None
        with WRITE_LOCK:
            if old is not None and path.exists():
                busy = True
            else:
                busy = False
                write_atomic(path, data)
                if old is not None and old.exists():
                    old.unlink()
        if busy:
            self.send_json(HTTPStatus.CONFLICT, {"error": f"term {path.stem!r} already exists"})
            return
        self.send_json(HTTPStatus.OK, {"id": path.stem, **data})

    def put_groups(self):
        """Replace the section list in meta.json. A section that still has terms cannot be removed."""
        try:
            groups = canonical_groups(self.read_object().get("groups"))
        except (ValueError, TypeError) as e:
            self.send_json(HTTPStatus.BAD_REQUEST, {"error": str(e)})
            return
        with WRITE_LOCK:
            meta = json.loads(META.read_text(encoding="utf-8"))
            kept = {g["id"] for g in groups}
            counts = term_counts_by_group()
            busy = [f"{g['id']} ({counts[g['id']]})" for g in meta.get("groups", [])
                    if g.get("id") not in kept and counts.get(g.get("id"))]
            if busy:
                self.send_json(HTTPStatus.CONFLICT, {"error": "section still has terms: " + ", ".join(busy)})
                return
            meta["groups"] = groups
            write_atomic(META, meta)
        self.send_json(HTTPStatus.OK, {"meta": meta})

    def do_DELETE(self):
        path = self.term_path()
        if path is None:
            return
        with WRITE_LOCK:
            if path.exists():
                path.unlink()
        self.send_json(HTTPStatus.OK, {"deleted": path.stem})


def main():
    ap = argparse.ArgumentParser(description="System1 glossary editor")
    ap.add_argument("--port", type=int, default=8000)
    args = ap.parse_args()
    TERMS.mkdir(exist_ok=True)
    httpd = ThreadingHTTPServer(("127.0.0.1", args.port), Handler)
    print(f"Glossary editor: http://127.0.0.1:{args.port}  (Ctrl+C to stop)")
    print(f"Terms are saved to {TERMS}")
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print()


if __name__ == "__main__":
    main()
