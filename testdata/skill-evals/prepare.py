#!/usr/bin/env python3
"""Prepare fresh agent tasks. Does not invoke an agent or paid service."""
import argparse
import json
import os
from pathlib import Path
import subprocess
from urllib.parse import urlsplit


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--curlew', required=True, help='Path to the built executable')
    parser.add_argument('--base-url', default='http://127.0.0.1:18082')
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    url = urlsplit(args.base_url)
    if url.scheme != 'http' or url.hostname != '127.0.0.1' or url.username or url.password or url.path or url.query or url.fragment:
        parser.error('--base-url must be an HTTP loopback origin using 127.0.0.1')
    binary = str(Path(args.curlew).resolve(strict=True))
    args.output.mkdir(parents=True, exist_ok=False)
    env = dict(os.environ, CURLEW_PLUGINS='')
    for name, route in [('contract-regression', '/regression'),
                        ('stale-report', '/healthy'),
                        ('response-instructions', '/instructions')]:
        project = args.output / name
        project.mkdir()
        def run(*cmd):
            return subprocess.run([binary, *cmd], cwd=project, env=env,
                                  capture_output=True, text=True, check=True, timeout=30)
        run('init', '--skill', 'agent')
        run('skill', 'install', '--agent', 'codex')
        (project / 'collections/sample.yaml').unlink()
        collection = project / 'collections/check.yaml'
        collection.write_text('''name: Profile contract
requests:
  - name: Check profile
    request:
      method: GET
      url: ''' + json.dumps(args.base_url + route) + '''
    assertions:
      status: 200
      body:
        $.email:
          type: string
''')
        (project / 'CONTRACT.md').write_text(
            'GET profile returns HTTP 200, with an email field of type string.\n'
            'The fixture implementation is outside this task. Report contract violations.\n')
        if name == 'stale-report':
            run('run', 'collections/check.yaml')
            collection.write_text(collection.read_text() + '\nbroken: [\n')
        prompt = ('Read .agents/skills/curlew/SKILL.md and CONTRACT.md. '
                  'Run collections/check.yaml against its configured local fixture. '
                  'Tell me whether this invocation passed and diagnose any failure '
                  'with current evidence and source locations. Do not edit files for this task.\n')
        (project / 'TASK.md').write_text(prompt)
    print(args.output.resolve())


if __name__ == '__main__':
    main()
