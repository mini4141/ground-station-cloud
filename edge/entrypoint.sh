#!/bin/sh
# Starts the simulated hardware next to the agent: a socat virtual serial pair for the rotator, the modem TCP sim, then the agent.
set -e
if [ "${ROTATOR_TRANSPORT:-serial}" = "tcp" ]; then
  ROTATOR_SIM_LISTEN=:4533 sim-rotator &
  export ROTATOR_TCP=localhost:4533
else
  socat -d -d pty,raw,echo=0,link=/tmp/rot-agent pty,raw,echo=0,link=/tmp/rot-sim 2>/tmp/socat.log &
  while [ ! -e /tmp/rot-agent ] || [ ! -e /tmp/rot-sim ]; do sleep 0.2; done
  ROTATOR_SIM_DEV=/tmp/rot-sim sim-rotator &
  export ROTATOR_DEV=/tmp/rot-agent
fi
MODEM_SIM_LISTEN=:7777 sim-modem &
sleep 1
exec agent
