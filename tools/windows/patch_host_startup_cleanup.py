"""Add orderly cleanup to an owned host's startup failure path; fail on drift."""
import argparse
import ast
from pathlib import Path


def transform(text):
    marker = 'lab.halt() # validates each owned PID identity before any signal\n'
    if text.count(marker) != 1 or text.count('def stop(*_):\n') != 1:
        raise ValueError('Unrecognized host layout; inspect before patching')
    start = text.index(marker)
    stop = text.index('def stop(*_):\n', start)
    signals = text.index('signal.signal(signal.SIGTERM,stop);signal.signal(signal.SIGINT,stop)\n', stop)
    loop = text.index('while True:\n', signals)
    body = text[stop:signals]
    if body.count(' raise SystemExit(0)\n') != 1:
        raise ValueError('Unrecognized shutdown handler')
    shutdown = body.replace('def stop(*_):\n', 'def shutdown():\n', 1).replace(' raise SystemExit(0)\n', '')
    handler = 'def stop(*_):\n shutdown()\n raise SystemExit(0)\n'
    startup = ''.join(' ' + line if line.strip() else line for line in text[start:stop].splitlines(keepends=True))
    result = (text[:start] + shutdown + handler + text[signals:loop]
              + 'try:\n' + startup + 'except BaseException:\n shutdown()\n raise\n' + text[loop:])
    ast.parse(result)
    return result


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('input', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    if args.output.exists():
        parser.error('Output exists; preserve existing files')
    args.output.write_text(transform(args.input.read_text()), encoding='utf-8')
