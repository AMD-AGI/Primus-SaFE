# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

"""Anthropic Messages API front for the SGLang model gateway.

POST /v1/messages and /v1/messages/count_tokens are translated to the OpenAI
chat completions API and sent to the gateway. SGLang's own AnthropicServing
converters build the requests, responses and streaming events, so the wire
format matches a standalone SGLang server. Every other request is forwarded
unchanged.
"""

import argparse
import logging
import sys
import time
import types

# serving.py imports one clock helper from a module that pulls in the model
# stack and initializes the GPU. The gateway container has no GPU, so a module
# with only that helper is registered before the import.
_stats = types.ModuleType("sglang.srt.observability.req_time_stats")
_stats.monotonic_time = time.perf_counter
sys.modules.setdefault(_stats.__name__, _stats)

import httpx
import uvicorn
from fastapi import FastAPI, Request
from fastapi.encoders import jsonable_encoder
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse, StreamingResponse
from sglang.srt.entrypoints.anthropic.protocol import (
    AnthropicCountTokensRequest,
    AnthropicCountTokensResponse,
    AnthropicMessagesRequest,
)
from sglang.srt.entrypoints.anthropic.serving import AnthropicServing
from sglang.srt.entrypoints.openai.protocol import ChatCompletionResponse
from starlette.background import BackgroundTask

logger = logging.getLogger("anthropic_proxy")

CHAT_PATH = "/v1/chat/completions"
HOP_BY_HOP = {
    "connection",
    "keep-alive",
    "proxy-authenticate",
    "proxy-authorization",
    "te",
    "trailers",
    "transfer-encoding",
    "upgrade",
    "host",
    "content-length",
}


def chat_body(chat_request) -> dict:
    """Serialize only the fields the converter set, so gateway defaults apply.

    Anthropic requests think only when asked to, so thinking defaults to off.
    """
    body = chat_request.model_dump(exclude_unset=True, exclude_none=True)
    body.setdefault("chat_template_kwargs", {}).setdefault("enable_thinking", False)
    return body


class GatewayChatBackend:
    """Stands in for OpenAIServingChat and talks to the gateway over HTTP."""

    # Inline system messages are merged by the converter.
    supports_inline_system = False

    def __init__(self, client: httpx.AsyncClient):
        self.client = client

    def supports_native_reasoning_history(self) -> bool:
        """Prior thinking blocks are sent as reasoning_content."""
        return True

    def wrap_reasoning_history(self, reasoning_text: str) -> str:
        """Unused because history is sent natively."""
        raise ValueError("reasoning history wrapping is not available")

    def apply_reasoning_enabled(self, request, enabled: bool) -> None:
        """Map Anthropic thinking on or off to the Qwen3 chat template switch."""
        request.chat_template_kwargs = {
            **(request.chat_template_kwargs or {}),
            "enable_thinking": enabled,
        }

    async def _generate_chat_stream(self, adapted_request, processed_request, raw_request):
        """Yield the gateway's chat completion SSE lines. Upstream errors become one data line."""
        body = chat_body(processed_request)
        body["stream"] = True
        async with self.client.stream("POST", CHAT_PATH, json=body) as resp:
            if resp.status_code != 200:
                yield "data: " + (await resp.aread()).decode("utf-8", errors="replace")
                return
            async for line in resp.aiter_lines():
                if line:
                    yield line + "\n\n"


class GatewayAnthropicServing(AnthropicServing):
    """AnthropicServing whose completions come from the gateway instead of an engine."""

    def __init__(self, client: httpx.AsyncClient):
        super().__init__(GatewayChatBackend(client))
        self.client = client

    async def _post_chat(self, chat_request, **overrides):
        """POST a chat request and return the raw upstream response."""
        body = chat_body(chat_request)
        body.update(overrides)
        return await self.client.post(CHAT_PATH, json=body)

    def _upstream_error(self, resp: httpx.Response) -> JSONResponse:
        """Convert a non-200 gateway response to an Anthropic error."""
        return self._convert_openai_error_response(
            types.SimpleNamespace(status_code=resp.status_code, body=resp.content)
        )

    async def _handle_non_streaming(self, chat_request, anthropic_request, raw_request):
        """Run one completion on the gateway and return an Anthropic message."""
        try:
            resp = await self._post_chat(chat_request, stream=False)
        except httpx.HTTPError as e:
            return self._error_response(502, "api_error", "Gateway unreachable", type(e).__name__)
        if resp.status_code != 200:
            return self._upstream_error(resp)
        response = ChatCompletionResponse.model_validate(resp.json())
        return JSONResponse(content=self._convert_response(response).model_dump(exclude_none=True))

    async def _handle_streaming(self, chat_request, anthropic_request, raw_request):
        """Stream Anthropic events converted from the gateway's SSE chunks."""
        return StreamingResponse(
            self._generate_anthropic_stream(None, chat_request, anthropic_request, raw_request),
            media_type="text/event-stream",
        )

    async def handle_count_tokens(self, request: AnthropicCountTokensRequest, raw_request: Request):
        """Count prompt tokens with a one-token completion on the gateway."""
        try:
            messages_request = AnthropicMessagesRequest(
                model=request.model,
                messages=request.messages,
                max_tokens=1,
                system=request.system,
                thinking=request.thinking,
                tools=request.tools,
                tool_choice=request.tool_choice,
            )
            chat_request = self._convert_to_chat_completion_request(messages_request)
        except Exception as e:
            return self._error_response(400, "invalid_request_error", str(e))
        try:
            resp = await self._post_chat(chat_request, stream=False, max_tokens=1)
        except httpx.HTTPError as e:
            return self._error_response(502, "api_error", "Gateway unreachable", type(e).__name__)
        if resp.status_code != 200:
            return self._upstream_error(resp)
        tokens = resp.json()["usage"]["prompt_tokens"]
        return JSONResponse(content=AnthropicCountTokensResponse(input_tokens=tokens).model_dump())


def build_app(upstream: str) -> FastAPI:
    """Create the proxy application for the given gateway base URL."""
    client = httpx.AsyncClient(
        base_url=upstream,
        timeout=httpx.Timeout(None, connect=10.0),
        limits=httpx.Limits(max_connections=1000, max_keepalive_connections=100),
    )
    serving = GatewayAnthropicServing(client)
    app = FastAPI(docs_url=None, redoc_url=None, openapi_url=None)

    @app.exception_handler(RequestValidationError)
    async def validation_error(request: Request, exc: RequestValidationError):
        if request.url.path.startswith("/v1/messages"):
            return serving._error_response(400, "invalid_request_error", str(exc.errors()[:3]))
        return JSONResponse(status_code=422, content={"detail": jsonable_encoder(exc.errors())})

    @app.post("/v1/messages")
    async def messages(request: AnthropicMessagesRequest, raw_request: Request):
        return await serving.handle_messages(request, raw_request)

    @app.post("/v1/messages/count_tokens")
    async def count_tokens(request: AnthropicCountTokensRequest, raw_request: Request):
        return await serving.handle_count_tokens(request, raw_request)

    @app.api_route(
        "/{path:path}",
        methods=["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"],
    )
    async def forward(path: str, request: Request):
        headers = {k: v for k, v in request.headers.items() if k.lower() not in HOP_BY_HOP}
        upstream_request = client.build_request(
            request.method,
            "/" + path,
            params=request.query_params,
            headers=headers,
            content=request.stream(),
        )
        try:
            resp = await client.send(upstream_request, stream=True)
        except httpx.HTTPError:
            return JSONResponse(status_code=503, content={"error": "gateway unreachable"})
        out_headers = {k: v for k, v in resp.headers.items() if k.lower() not in HOP_BY_HOP}
        return StreamingResponse(
            resp.aiter_raw(),
            status_code=resp.status_code,
            headers=out_headers,
            background=BackgroundTask(resp.aclose),
        )

    return app


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="0.0.0.0")
    parser.add_argument("--port", type=int, default=8000)
    parser.add_argument("--upstream", default="http://127.0.0.1:8001")
    args = parser.parse_args()
    logging.basicConfig(level=logging.INFO)
    uvicorn.run(build_app(args.upstream), host=args.host, port=args.port, log_level="info")


if __name__ == "__main__":
    main()
