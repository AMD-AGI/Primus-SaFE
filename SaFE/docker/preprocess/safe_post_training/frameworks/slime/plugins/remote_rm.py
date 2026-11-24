#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""Remote reward with a key: slime's remote_rm request body and retries, plus an Authorization: Bearer header.

The key comes from PT_RM_API_KEY, which the run's secret provides as an environment variable; it never appears in the
arguments. A rejected key (HTTP 401 or 403) is not retried: the plugin prints REWARD_REJECTED_TEXT, which the launcher
turns into a failure that is not retried either.
"""

import asyncio
import logging
import os
import random

from safe_post_training.stages import REWARD_REJECTED_TEXT

KEY_ENV = "PT_RM_API_KEY"
MAX_RETRIES = 10
REJECTED_STATUSES = (401, 403)

logger = logging.getLogger(__name__)
_session = None


class RewardKeyRejected(Exception):
    """The reward service refused the key."""


def _get_session():
    """Return the shared HTTP session, created on first use like upstream's."""
    global _session
    if _session is None or _session.closed:
        import aiohttp

        _session = aiohttp.ClientSession(connector=aiohttp.TCPConnector(limit=64, enable_cleanup_closed=True),
                                         timeout=aiohttp.ClientTimeout(total=120))
    return _session


async def remote_rm(args, sample, max_retries=MAX_RETRIES, **kwargs):
    """Score one sample with the service at args.rm_url and return its JSON answer."""
    payload = {"prompt": sample.prompt, "response": sample.response, "label": sample.label}
    headers = {"Authorization": "Bearer " + os.environ.get(KEY_ENV, "")}
    session = _get_session()
    for attempt in range(max_retries):
        try:
            async with session.post(args.rm_url, json=payload, headers=headers) as resp:
                if resp.status in REJECTED_STATUSES:
                    print("%s (HTTP %d)" % (REWARD_REJECTED_TEXT, resp.status), flush=True)
                    raise RewardKeyRejected("the reward service answered HTTP %d" % resp.status)
                resp.raise_for_status()
                return await resp.json()
        except RewardKeyRejected:
            raise
        except Exception as e:
            if attempt + 1 >= max_retries:
                logger.warning("remote_rm failed after %d attempts: %s", attempt + 1, e)
                raise
            backoff = min(2 ** attempt, 30) + random.random()
            logger.info("remote_rm: %s, retrying in %.1fs (%d/%d)", type(e).__name__, backoff, attempt + 1,
                        max_retries)
            await asyncio.sleep(backoff)
