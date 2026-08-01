#!/usr/bin/env python3
"""End-to-end test for persistence, recovery, rollback, storm and replay APIs."""

from __future__ import annotations

import json
import os
import pathlib
import subprocess
import sys
import time
import urllib.request


ROOT = pathlib.Path(__file__).resolve().parent.parent
IS_WINDOWS = os.name == "nt"
SUFFIX = ".exe" if IS_WINDOWS else ""
API = "http://127.0.0.1:28080"


def request_json(path: str, method: str = "GET", body: dict | None = None, timeout: int = 3) -> dict:
    data = json.dumps(body).encode() if body is not None else None
    request = urllib.request.Request(
        API + path,
        data=data,
        method=method,
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(request, timeout=timeout) as response:
        return json.loads(response.read().decode())


def wait_health(processes: list[subprocess.Popen]) -> None:
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        if any(process.poll() is not None for process in processes):
            raise RuntimeError("a service exited during startup")
        try:
            if request_json("/healthz", timeout=1).get("brain") == "ok":
                return
        except Exception:
            pass
        time.sleep(0.2)
    raise RuntimeError("services did not become healthy")


def wait_incident(service: str, terminal: str) -> dict:
    deadline = time.monotonic() + 25
    while time.monotonic() < deadline:
        incidents = request_json("/api/v1/incidents?page_size=500").get("incidents", [])
        incident = next((item for item in incidents if item.get("service") == service), None)
        if incident and incident.get("status") == terminal:
            return incident
        time.sleep(0.25)
    raise RuntimeError(f"{service} did not reach {terminal}")


def main() -> int:
    cache = ROOT / ".cache" / "e2e-suite" / str(int(time.time() * 1000))
    cache.mkdir(parents=True, exist_ok=True)
    env = os.environ.copy()
    env.update(
        {
            "WATCHTOWER_PORT": "28080",
            "WATCHTOWER_BRAIN_ADDR": "127.0.0.1:25052",
            "WATCHTOWER_MOCK_CONTROL_URL": "http://127.0.0.1:28081",
            "WATCHTOWER_DB_PATH": str(cache / "watchtower.db"),
            "BRAIN_GRPC_PORT": "25052",
            "MOCK_PROMETHEUS_PORT": "29090",
            "MOCK_K8S_PORT": "28081",
            "MOCK_PROMETHEUS_URL": "http://127.0.0.1:29090",
            "MOCK_K8S_URL": "http://127.0.0.1:28081",
            "LLM_ENABLED": "false",
            "AUTOFIX_OBSERVE_SECONDS": "1",
        }
    )
    python_bin = os.environ.get("BRAIN_PYTHON", sys.executable)
    processes: list[subprocess.Popen] = []
    log_handles = []

    def start(command: list[str], name: str) -> subprocess.Popen:
        stdout = open(cache / f"{name}.out.log", "w", encoding="utf-8")
        stderr = open(cache / f"{name}.err.log", "w", encoding="utf-8")
        log_handles.extend([stdout, stderr])
        flags = subprocess.CREATE_NEW_PROCESS_GROUP if IS_WINDOWS else 0
        process = subprocess.Popen(command, cwd=ROOT, env=env, stdout=stdout, stderr=stderr, creationflags=flags)
        processes.append(process)
        return process

    try:
        start([str(ROOT / "bin" / f"mock-infra{SUFFIX}")], "mock")
        start([python_bin, str(ROOT / "brain" / "server.py")], "brain")
        gateway = start([str(ROOT / "bin" / f"gateway{SUFFIX}")], "gateway")
        wait_health(processes)

        request_json("/api/v1/mock/trigger", "POST", {"scenario": "crash_loop", "service": "e2e-success", "repair_outcome": "success"})
        success = wait_incident("e2e-success", "resolved")
        assert "最终结论" in success.get("conclusion", "")

        request_json("/api/v1/mock/trigger", "POST", {"scenario": "crash_loop", "service": "e2e-failure", "repair_outcome": "failure"})
        failure = wait_incident("e2e-failure", "escalated")
        assert "回滚" in failure.get("conclusion", "")

        success_timeline = request_json(f"/api/v1/incidents/{success['id']}/timeline").get("timeline", [])
        failure_timeline = request_json(f"/api/v1/incidents/{failure['id']}/timeline").get("timeline", [])
        assert any(item["event_type"] == "incident_resolved" for item in success_timeline)
        assert any(item["event_type"] == "autofix_rollback" for item in failure_timeline)
        replay = request_json(f"/api/v1/incidents/{success['id']}/replay")
        assert replay.get("frame_count") == len(success_timeline) and replay["frame_count"] > 5

        storm = request_json("/api/v1/mock/storm", "POST", {"count": 300, "duplicate_ratio": 0.9, "service": "e2e-storm"})
        assert storm["deduplicated"] > storm["accepted"]

        gateway.terminate()
        gateway.wait(timeout=5)
        processes.remove(gateway)
        gateway = start([str(ROOT / "bin" / f"gateway{SUFFIX}")], "gateway-restarted")
        wait_health(processes)
        restored = request_json(f"/api/v1/incidents/{success['id']}")
        assert restored["status"] == "resolved" and restored["conclusion"] == success["conclusion"]

        print(json.dumps({
            "persistence": "ok", "timeline": len(success_timeline), "replay": replay["frame_count"],
            "rollback": "ok", "storm_requested": storm["requested"], "storm_deduplicated": storm["deduplicated"],
        }, ensure_ascii=False))
        return 0
    finally:
        for process in reversed(processes):
            if process.poll() is None:
                process.terminate()
        for process in reversed(processes):
            try:
                process.wait(timeout=4)
            except subprocess.TimeoutExpired:
                process.kill()
        for handle in log_handles:
            handle.close()


if __name__ == "__main__":
    raise SystemExit(main())
