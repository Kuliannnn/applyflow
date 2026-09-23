"""Test embedded migrations in a disposable PostgreSQL cluster, never an existing DB.

Requires local PostgreSQL binaries (PG_BIN or pg_config). Uses only a private Unix
socket. If shutdown fails, retain the directory rather than delete live PG files.
"""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
from urllib.parse import urlencode

ROOT = Path(__file__).resolve().parents[1]


def postgres_bin():
    configured = os.environ.get("PG_BIN")
    if not configured:
        if not shutil.which("pg_config"):
            raise SystemExit("Install PostgreSQL or set PG_BIN to its bin directory.")
        configured = subprocess.check_output(["pg_config", "--bindir"], text=True).strip()
    bindir = Path(configured)
    for tool in ["initdb", "pg_ctl", "createdb", "postgres"]:
        if not (bindir / tool).is_file():
            raise SystemExit(f"Missing PostgreSQL executable: {tool}")
    return bindir


def main():
    bindir = postgres_bin()
    base = Path(tempfile.mkdtemp(prefix="applyflow-pg-", dir="/tmp"))
    data, sock, log = base / "data", base / "socket", base / "server.log"
    attempted_start = False
    try:
        sock.mkdir(mode=0o700)
        subprocess.run(
            [str(bindir / "initdb"), "-D", str(data), "-U", "applyflow_test",
             "--auth=trust", "--encoding=UTF8", "--no-locale"],
            check=True, stdout=subprocess.DEVNULL,
        )
        attempted_start = True
        subprocess.run(
            [str(bindir / "pg_ctl"), "-D", str(data), "-l", str(log),
             "-o", f"-k {sock} -c listen_addresses=''", "-w", "start"],
            check=True, stdout=subprocess.DEVNULL,
        )
        subprocess.run(
            [str(bindir / "createdb"), "-h", str(sock), "-U", "applyflow_test", "applyflow_test"],
            check=True,
        )
        env = dict(os.environ)
        dsn = "postgresql://applyflow_test@/applyflow_test?" + urlencode({"host": str(sock), "sslmode": "disable"})
        env.update(TEST_DATABASE_URL=dsn, DATABASE_URL=dsn)
        print(subprocess.check_output([str(bindir / "postgres"), "--version"], text=True).strip(), flush=True)
        # Exercise the actual command, including its session lock and embedded FS.
        for command in ["status", "up", "up", "status"]:
            subprocess.run(["go", "run", "./cmd/migrate", command], cwd=ROOT / "backend", env=env, check=True)
        subprocess.run(
            ["go", "test", "-count=1", "-v", "./tests/integration"],
            cwd=ROOT / "backend", env=env, check=True,
        )
    except subprocess.CalledProcessError:
        if log.exists():
            print(log.read_text())
        raise
    finally:
        if attempted_start:
            status = subprocess.run(
                [str(bindir / "pg_ctl"), "-D", str(data), "status"],
                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
            )
            if status.returncode == 0:
                stopped = subprocess.run(
                    [str(bindir / "pg_ctl"), "-D", str(data), "-m", "immediate", "-w", "stop"],
                    stdout=subprocess.DEVNULL,
                )
                if stopped.returncode:
                    raise RuntimeError(f"PostgreSQL did not stop; retained {base}")
            elif status.returncode != 3:
                raise RuntimeError(f"Cannot confirm PostgreSQL stopped; retained {base}")
        shutil.rmtree(base)


if __name__ == "__main__":
    main()
