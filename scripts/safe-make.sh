#!/bin/bash
# Block dangerous make targets when running as a Botka task agent.
# This script wraps `make` and prevents targets that would restart
# or deploy the botka service, which would kill the running agent.

if [ -z "$BOTKA_TASK_AGENT" ]; then
    exec make "$@"
fi

BLOCKED_TARGETS="deploy install-service"
for target in $BLOCKED_TARGETS; do
    for arg in "$@"; do
        if [ "$arg" = "$target" ]; then
            echo "ERROR: 'make $target' is blocked inside task agents to prevent self-restart." >&2
            echo "If deployment is needed, commit your changes and note it in the task output." >&2
            exit 1
        fi
    done
done
exec make "$@"
