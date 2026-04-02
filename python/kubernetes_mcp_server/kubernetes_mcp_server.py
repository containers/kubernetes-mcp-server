import os
import platform
import signal
import subprocess
import sys
from importlib.metadata import version
from pathlib import Path
import shutil
import tempfile
import urllib.request

__version__ = version("kubernetes-mcp-server")

def get_platform_binary():
    """Determine the correct binary for the current platform."""
    system = platform.system().lower()
    arch = platform.machine().lower()

    # Normalize architecture names
    if arch in ["x86_64", "amd64"]:
        arch = "amd64"
    elif arch in ["arm64", "aarch64"]:
        arch = "arm64"
    else:
        raise RuntimeError(f"Unsupported architecture: {arch}")

    if system == "darwin":
        return f"kubernetes-mcp-server-darwin-{arch}"
    elif system == "linux":
        return f"kubernetes-mcp-server-linux-{arch}"
    elif system == "windows":
        return f"kubernetes-mcp-server-windows-{arch}.exe"
    else:
        raise RuntimeError(f"Unsupported operating system: {system}")

def download_binary(binary_version="latest", destination=None):
    """Download the correct binary for the current platform."""
    binary_name = get_platform_binary()
    if destination is None:
        destination = Path.home() / ".kubernetes-mcp-server" / "bin" / binary_version

    destination = Path(destination)
    destination.mkdir(parents=True, exist_ok=True)
    binary_path = destination / binary_name

    if binary_path.exists():
        return binary_path

    base_url = "https://github.com/containers/kubernetes-mcp-server/releases"
    if binary_version == "latest":
        release_url = f"{base_url}/latest/download/{binary_name}"
    else:
        release_url = f"{base_url}/download/v{binary_version}/{binary_name}"

    # Download the binary
    print(f"Downloading {binary_name} from {release_url}")
    with tempfile.NamedTemporaryFile(delete=False) as temp_file:
        try:
            with urllib.request.urlopen(release_url) as response:
                shutil.copyfileobj(response, temp_file)
            temp_file.close()

            # Move to destination and make executable
            shutil.move(temp_file.name, binary_path)
            binary_path.chmod(binary_path.stat().st_mode | 0o755)  # Make executable

            return binary_path
        except Exception as e:
            os.unlink(temp_file.name)
            raise RuntimeError(f"Failed to download binary: {e}")

def execute(args=None):
    """Download and execute the kubernetes-mcp-server binary."""
    if args is None:
        args = []

    try:
        binary_path = download_binary(binary_version=__version__)
        cmd = [str(binary_path)] + args

        # Execute the binary with the provided arguments
        process = subprocess.Popen(cmd)

        def handle_signal(signum, frame):
            try:
                process.send_signal(signum)
            except OSError:
                pass

        signal.signal(signal.SIGTERM, handle_signal)
        if hasattr(signal, "SIGHUP"):
            signal.signal(signal.SIGHUP, handle_signal)

        try:
            return process.wait()
        except KeyboardInterrupt:
            try:
                process.send_signal(signal.SIGINT)
            except OSError:
                pass
            return process.wait()
    except Exception as e:
        print(f"Error executing kubernetes-mcp-server: {e}", file=sys.stderr)
        return 1

if __name__ == "__main__":
    sys.exit(execute(sys.argv[1:]))


def main():
    """Main function to execute the kubernetes-mcp-server binary."""
    args = sys.argv[1:] if len(sys.argv) > 1 else []
    return execute(args)
