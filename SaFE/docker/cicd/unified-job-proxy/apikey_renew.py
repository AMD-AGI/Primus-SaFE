#!/usr/bin/env python3

#  Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
#  See LICENSE for license information.

"""Ensure USER_APIKEY is valid against identity when IDENTITY_URL is set.

Calls GET /api/v1/apikeys/current with the key. If the key expires within
RENEW_WITHIN_DAYS days, renews it with POST /api/v1/apikeys/{id}/renew.
The plaintext key does not change on renew.
"""

from __future__ import annotations

import os
import sys
from datetime import datetime, timezone
from typing import Any, Dict, Optional

import requests

IDENTITY_URL_ENV = "IDENTITY_URL"
USER_APIKEY_ENV = "USER_APIKEY"
RENEW_WITHIN_DAYS = 3
RENEW_TTL_DAYS = 365
SAFE_TIME_LAYOUT = "%Y-%m-%dT%H:%M:%S"


def _identity_base() -> Optional[str]:
    raw = (os.environ.get(IDENTITY_URL_ENV) or "").strip().rstrip("/")
    return raw or None


def _parse_expiration(value: Any) -> Optional[datetime]:
    if not isinstance(value, str) or not value.strip():
        return None
    text = value.strip()
    for fmt in (SAFE_TIME_LAYOUT, "%Y-%m-%dT%H:%M:%SZ", "%Y-%m-%dT%H:%M:%S%z"):
        try:
            dt = datetime.strptime(text, fmt)
            if dt.tzinfo is None:
                return dt.replace(tzinfo=timezone.utc)
            return dt.astimezone(timezone.utc)
        except ValueError:
            continue
    return None


def ensure_user_apikey(session: Optional[requests.Session] = None) -> None:
    """Validate and optionally renew USER_APIKEY via identity. No-op without IDENTITY_URL."""
    base = _identity_base()
    if not base:
        return
    api_key = (os.environ.get(USER_APIKEY_ENV) or "").strip()
    if not api_key:
        raise RuntimeError(f"{USER_APIKEY_ENV} is empty while {IDENTITY_URL_ENV} is set")

    own_session = session is None
    s = session or requests.Session()
    headers = {"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"}
    try:
        url = f"{base}/api/v1/apikeys/current"
        resp = s.get(url, headers=headers, timeout=30)
        if resp.status_code >= 300:
            raise RuntimeError(
                f"API key check failed: HTTP {resp.status_code} {resp.text}"
            )
        data: Dict[str, Any] = resp.json()
        key_id = data.get("id")
        exp = _parse_expiration(data.get("expirationTime"))
        if exp is None:
            print("[warn] identity current key has no expirationTime; skipping renew", file=sys.stderr)
            return
        now = datetime.now(timezone.utc)
        if exp <= now:
            raise RuntimeError("API key expired")
        remaining = exp - now
        if remaining.total_seconds() > RENEW_WITHIN_DAYS * 24 * 3600:
            return
        if key_id is None:
            raise RuntimeError("identity current key has no id; cannot renew")
        renew_url = f"{base}/api/v1/apikeys/{int(key_id)}/renew"
        renew_resp = s.post(
            renew_url,
            headers=headers,
            json={"ttlDays": RENEW_TTL_DAYS},
            timeout=30,
        )
        if renew_resp.status_code >= 300:
            raise RuntimeError(
                f"API key renew failed: HTTP {renew_resp.status_code} {renew_resp.text}"
            )
        print(
            f"[info] renewed API key id={key_id} for {RENEW_TTL_DAYS} days "
            f"(was expiring in {remaining.days}d)",
            file=sys.stderr,
        )
    finally:
        if own_session:
            s.close()
