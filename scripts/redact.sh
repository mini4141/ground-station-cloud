#!/usr/bin/env bash
# stdin -> stdout: redacts AWS account ids, access keys, JWTs/tokens, passwords and public IPs before proof is saved.
sed -E \
  -e 's/\b[0-9]{12}\b/<ACCOUNT_ID>/g' \
  -e 's/(AKIA|ASIA)[A-Z0-9]{16}/<ACCESS_KEY>/g' \
  -e 's/eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]*/<JWT>/g' \
  -e 's/(password|secret|token)(["= :]+)[^", ]{6,}/\1\2<REDACTED>/Ig' \
  -e "s/${DEPLOYER_IP:-__no_ip__}/<DEPLOYER_IP>/g"
