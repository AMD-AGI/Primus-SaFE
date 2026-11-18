#!/usr/bin/env python3

#  Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
#  See LICENSE for license information.

"""Renew USER_APIKEY against identity when IDENTITY_URL is set and the key is near expiry.

Calls GET /api/v1/apikeys/current with the key. If the key expires within
RENEW_WITHIN_DAYS days, renews it with POST /api/v1/apikeys/{id}/renew.
The plaintext key does not change on renew. The check is best-effort: any
failure is logged and the caller continues with the key it has.
Shared by runner-proxy and unified-job-proxy; copied into both images.
"""

from __future__ import annotations

import os
import re
import sys
from datetime import datetime, timezone
from typing import Any, Dict, Optional

import requests

IDENTITY_URL_ENV = "IDENTITY_URL"
USER_APIKEY_ENV = "USER_APIKEY"
RENEW_WITHIN_DAYS = 3
RENEW_TTL_DAYS = 365
REQUEST_TIMEOUT_SECONDS = 15

_RFC3339 = re.compile(
    r"^(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2}:\d{2})(?:\.(\d+))?(Z|[+-]\d{2}:?\d{2})?$",
    re.IGNORECASE,
)


def _warn(message: str) -> None:
    """Print a warning to stderr."""
    print(f"[warn] {message}", file=sys.stderr)


def _identity_base() -> Optional[str]:
    """Return IDENTITY_URL without a trailing slash, or None when unset."""
    raw = (os.environ.get(IDENTITY_URL_ENV) or "").strip().rstrip("/")
    return raw or None


def _parse_expiration(value: Any) -> Optional[datetime]:
    """Parse an RFC3339 timestamp (fractional seconds and offset optional) as UTC; None if invalid."""
    if not isinstance(value, str):
        return None
    m = _RFC3339.match(value.strip())
    if not m:
        return None
    date, clock, fraction, zone = m.groups()
    micros = (fraction or "")[:6].ljust(6, "0")
    offset = "+0000" if not zone or zone.upper() == "Z" else zone.replace(":", "")
    try:
        dt = datetime.strptime(f"{date}T{clock}.{micros}{offset}", "%Y-%m-%dT%H:%M:%S.%f%z")
    except ValueError:
        return None
    return dt.astimezone(timezone.utc)


def _renew_if_near_expiry(s: requests.Session, base: str, headers: Dict[str, str]) -> None:
    """Read the current key and renew it when it expires within RENEW_WITHIN_DAYS days."""
    resp = s.get(f"{base}/api/v1/apikeys/current", headers=headers, timeout=REQUEST_TIMEOUT_SECONDS)
    if resp.status_code >= 300:
        _warn(f"API key check returned HTTP {resp.status_code}; skipping renew")
        return
    data: Dict[str, Any] = resp.json()
    exp = _parse_expiration(data.get("expirationTime"))
    if exp is None:
        _warn("identity current key has no valid expirationTime; skipping renew")
        return
    remaining = exp - datetime.now(timezone.utc)
    if remaining.total_seconds() <= 0:
        _warn("API key has expired; skipping renew")
        return
    if remaining.total_seconds() > RENEW_WITHIN_DAYS * 24 * 3600:
        return
    key_id = data.get("id")
    if isinstance(key_id, bool) or not isinstance(key_id, int):
        _warn("identity current key has no numeric id; skipping renew")
        return
    renew_resp = s.post(
        f"{base}/api/v1/apikeys/{key_id}/renew",
        headers=headers,
        json={"ttlDays": RENEW_TTL_DAYS},
        timeout=REQUEST_TIMEOUT_SECONDS,
    )
    if renew_resp.status_code >= 300:
        _warn(f"API key renew returned HTTP {renew_resp.status_code}")
        return
    print(
        f"[info] renewed API key id={key_id} for {RENEW_TTL_DAYS} days "
        f"(was expiring in {remaining.days}d)",
        file=sys.stderr,
    )


def ensure_user_apikey(session: Optional[requests.Session] = None) -> None:
    """Renew USER_APIKEY near expiry via identity. No-op without IDENTITY_URL; never raises."""
    base = _identity_base()
    if not base:
        return
    api_key = (os.environ.get(USER_APIKEY_ENV) or "").strip()
    if not api_key:
        _warn(f"{USER_APIKEY_ENV} is empty; skipping renew")
        return
    own_session = session is None
    s = session or requests.Session()
    headers = {"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"}
    try:
        _renew_if_near_expiry(s, base, headers)
    except Exception as exc:  # best-effort: identity problems must not fail the job
        _warn(f"API key renew check failed: {exc}")
    finally:
        if own_session:
            s.close()
