"""Exercise the packaged v1 ABI with a temporary C++ project."""
import ctypes
import json
import sys
import tempfile
from pathlib import Path

package = Path(sys.argv[1]).resolve()
manifest = json.loads((package / "engine.json").read_text())
library = ctypes.CDLL(str(package / manifest["library"]))


class View(ctypes.Structure):
    _fields_ = [("data", ctypes.c_char_p), ("size", ctypes.c_size_t)]


class Buffer(ctypes.Structure):
    _fields_ = [("data", ctypes.c_void_p), ("size", ctypes.c_size_t)]


class Result(ctypes.Structure):
    _fields_ = [("status", ctypes.c_int32), ("response", Buffer)]


library.execute_v1.argtypes = [View]
library.execute_v1.restype = Result
library.cancel_v1.argtypes = [View]
library.cancel_v1.restype = ctypes.c_bool
library.release_buffer_v1.argtypes = [Buffer]
library.release_buffer_v1.restype = None
with tempfile.TemporaryDirectory() as directory:
    root = Path(directory).resolve()
    (root / "main.cpp").write_text("int main() { return 0; }\n")
    request = json.dumps({
        "protocolVersion": 1, "requestId": "package-check", "stack": "cpp",
        "operation": "scan", "scan": {"repository": str(root), "facts": ["files"], "static": True},
    }).encode()
    result = library.execute_v1(View(request, len(request)))
    try:
        response = json.loads(ctypes.string_at(result.response.data, result.response.size))
        assert result.status == 0 and response["error"] is None, response
        assert any(item["path"] == "main.cpp" for item in response["report"]["facts"]["files"]["files"])
    finally:
        library.release_buffer_v1(result.response)


with tempfile.TemporaryDirectory() as directory:
    root = Path(directory).resolve()
    before, after = root / "before", root / "after"
    before.mkdir()
    after.mkdir()
    (before / "main.cpp").write_text("int changed() { return 1; }\n")
    (after / "main.cpp").write_text("int changed() { return 2; }\n")
    request = json.dumps({
        "protocolVersion": 1, "requestId": "impact-package-check", "stack": "cpp",
        "operation": "impact", "impact": {
            "before": "a" * 40, "after": "b" * 40,
            "beforeRoot": str(before), "afterRoot": str(after),
        },
    }).encode()
    result = library.execute_v1(View(request, len(request)))
    try:
        response = json.loads(ctypes.string_at(result.response.data, result.response.size))
        assert result.status == 0 and response["error"] is None, response
        left = response["report"]["before"]["symbols"][0]
        right = response["report"]["after"]["symbols"][0]
        assert left["key"] == right["key"] and left["fingerprint"] != right["fingerprint"]
    finally:
        library.release_buffer_v1(result.response)
