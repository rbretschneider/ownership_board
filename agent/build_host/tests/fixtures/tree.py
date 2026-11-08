"""Ordinary child/grandchild tree; deepest child ignores graceful termination."""
import os
from pathlib import Path
import signal
import subprocess
import sys
import time

path = Path(sys.argv[1])
depth = int(sys.argv[2]) if len(sys.argv)>2 else 0
with path.open('a') as output:
    output.write(str(os.getpid())+'\n')
if depth < 2:
    subprocess.Popen([sys.executable,__file__,str(path),str(depth+1)])
else:
    signal.signal(signal.SIGTERM, signal.SIG_IGN)
while True:
    time.sleep(.1)
