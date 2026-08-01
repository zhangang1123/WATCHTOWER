#!/usr/bin/env python3
"""Cross-platform process supervisor for local Watchtower development."""

from __future__ import annotations

import os
import shutil
import signal
import socket
import subprocess
import sys
import time
from dataclasses import dataclass
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
IS_WINDOWS = os.name == "nt"


def load_config() -> None:
    path = Path(os.environ.get("WATCHTOWER_CONFIG_FILE", ROOT / ".env"))
    if not path.is_absolute():
        path = ROOT / path
    if not path.exists():
        return
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[7:].lstrip()
        key, separator, value = line.partition("=")
        if not separator or not key.strip():
            continue
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in {"'", '"'}:
            value = value[1:-1]
        os.environ.setdefault(key.strip(), value)


def env(name: str, default: str) -> str:
    return os.environ.get(name, default)


@dataclass
class Service:
    name: str
    process: subprocess.Popen


services: list[Service] = []
stopping = False


def wait_for_ports(name: str, process: subprocess.Popen, ports: list[int], timeout: float = 20.0) -> None:
    deadline = time.monotonic() + timeout
    pending = set(ports)
    while pending and time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError(f"{name} exited during startup with code {process.returncode}")
        for port in tuple(pending):
            try:
                with socket.create_connection(("127.0.0.1", port), timeout=0.3):
                    pending.remove(port)
            except OSError:
                pass
        if pending:
            time.sleep(0.2)
    if pending:
        raise RuntimeError(f"{name} did not open ports: {sorted(pending)}")


def validate_ports(named_ports: dict[str, int]) -> None:
    seen: dict[int, str] = {}
    for name, port in named_ports.items():
        if not 1 <= port <= 65535:
            raise RuntimeError(f"{name} port is outside 1-65535: {port}")
        if port in seen:
            raise RuntimeError(f"configured port conflict: {name} and {seen[port]} both use {port}")
        seen[port] = name
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as probe:
            probe.settimeout(0.3)
            if probe.connect_ex(("127.0.0.1", port)) == 0:
                raise RuntimeError(f"{name} port {port} is already occupied")


def start_service(
    name: str,
    command: list[str],
    ports: list[int],
    *,
    cwd: Path = ROOT,
    env: dict[str, str] | None = None,
) -> None:
    process_env = os.environ.copy()
    if env:
        process_env.update(env)
    kwargs: dict = {"cwd": str(cwd), "env": process_env}
    if IS_WINDOWS:
        kwargs["creationflags"] = subprocess.CREATE_NEW_PROCESS_GROUP
    else:
        kwargs["start_new_session"] = True

    process = subprocess.Popen(command, **kwargs)
    service = Service(name, process)
    services.append(service)
    print(f"\033[0;34m[watchtower]\033[0m {name} 已启动 (PID {process.pid})", flush=True)
    wait_for_ports(name, process, ports)


def terminate_service(service: Service) -> None:
    process = service.process
    if process.poll() is not None:
        return
    if IS_WINDOWS:
        subprocess.run(
            ["taskkill", "/PID", str(process.pid), "/T", "/F"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            check=False,
        )
    else:
        try:
            os.killpg(process.pid, signal.SIGTERM)
            process.wait(timeout=3)
        except (ProcessLookupError, subprocess.TimeoutExpired):
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
    try:
        process.wait(timeout=2)
    except subprocess.TimeoutExpired:
        process.kill()
    print(f"\033[0;34m[watchtower]\033[0m {service.name} 已停止", flush=True)


def stop_all(*_: object) -> None:
    global stopping
    if stopping:
        return
    stopping = True
    print("\n\033[0;34m[watchtower]\033[0m 正在停止全部服务...", flush=True)
    for service in reversed(services):
        terminate_service(service)


def executable(relative_path: str) -> str:
    path = ROOT / relative_path
    if path.exists():
        return str(path)
    windows_path = path.with_suffix(".exe")
    if windows_path.exists():
        return str(windows_path)
    raise RuntimeError(f"missing executable: {path}")


def main() -> int:
    load_config()
    web_port = int(env("WATCHTOWER_WEB_PORT", "3000"))
    gateway_port = int(env("WATCHTOWER_PORT", "8080"))
    brain_port = int(env("BRAIN_GRPC_PORT", "50052"))
    prometheus_port = int(env("MOCK_PROMETHEUS_PORT", "9090"))
    k8s_port = int(env("MOCK_K8S_PORT", "8081"))
    validate_ports({
        "Web Console": web_port,
        "Gateway": gateway_port,
        "Brain": brain_port,
        "Mock Prometheus": prometheus_port,
        "Mock K8s": k8s_port,
    })
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, stop_all)

    node = shutil.which("node")
    if not node:
        raise RuntimeError("node is not available")

    try:
        start_service(
            "Mock Infra",
            [executable("bin/mock-infra")],
            [prometheus_port, k8s_port],
        )
        start_service(
            "Brain",
            [sys.executable, str(ROOT / "brain/server.py")],
            [brain_port],
        )
        start_service(
            "Gateway",
            [executable("bin/gateway")],
            [gateway_port],
        )
        start_service(
            "Web Console",
            [node, str(ROOT / "web/node_modules/vite/bin/vite.js"), "--host", "0.0.0.0"],
            [web_port],
            cwd=ROOT / "web",
        )

        print(
            "\n\033[0;32mWatchtower-Lite 已就绪\033[0m\n"
            f"  控制台:  http://localhost:{web_port}\n"
            f"  API:     http://localhost:{gateway_port}\n"
            f"  Brain:   127.0.0.1:{brain_port}\n\n"
            "按 Ctrl+C 停止全部服务。",
            flush=True,
        )

        while not stopping:
            for service in services:
                code = service.process.poll()
                if code is not None:
                    raise RuntimeError(f"{service.name} exited unexpectedly with code {code}")
            time.sleep(0.25)
        return 0
    except KeyboardInterrupt:
        return 0
    except Exception as error:
        print(f"\033[0;31m[watchtower] {error}\033[0m", file=sys.stderr, flush=True)
        return 1
    finally:
        stop_all()


if __name__ == "__main__":
    raise SystemExit(main())
