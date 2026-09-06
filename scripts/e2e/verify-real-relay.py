#!/usr/bin/env python3
"""Exercise three relay wire APIs; this command makes model POSTs, not fixtures.

Required environment: RELAY_BASE_URL (origin or /v1 base), RELAY_API_KEY,
RELAY_MODEL. No service discovery, provisioning, retries, or built-in tools.
Requires system curl >= 8.4 for unknown-length transfer-size enforcement.
Each protocol uses at most four POSTs: JSON, SSE, echo call, echo followup.
Only metadata is printed. Bodies, prompts, tool payloads and auth stay in memory.
A pass proves client-visible relay semantics, NOT upstream/provider provenance.
The companion unittest file contains validator tests, never live relay proof.
"""

import argparse
import json
import os
import secrets
import subprocess
import sys
from urllib.parse import urlsplit

PROTOCOLS = ("chat", "responses", "messages")
PATHS = {"chat": "/chat/completions", "responses": "/responses", "messages": "/messages"}
TOOL_NAME = "metapi_echo"
MAX_BODY = 1024 * 1024
MAX_EVENTS = 10000
HTTP_MARKER = b"\n__METAPI_HTTP__"
SUMMARY_FIELDS = ("model", "responseId", "usage", "eventCount", "contentLength")
USAGE_FIELDS = (
    "input_tokens", "output_tokens", "total_tokens", "prompt_tokens",
    "completion_tokens", "cache_read_input_tokens", "cache_creation_input_tokens",
)


class CheckFailed(Exception):
    """Only constant, non-payload error codes may leave the validator."""


def require(condition, code):
    if not condition:
        raise CheckFailed(code)


def identifier(value):
    require(isinstance(value, str) and 0 < len(value) <= 256
            and all(c.isprintable() and not c.isspace() for c in value), "invalid_identifier")
    return value


def integer(value):
    return type(value) is int and value >= 0


def has_text(value):
    return any(c.isprintable() and not c.isspace() for c in value)


def no_error(obj):
    require(isinstance(obj, dict), "expected_object")
    require(obj.get("error") is None and obj.get("incomplete_details") is None
            and obj.get("type") not in ("error", "response.failed", "response.incomplete")
            and obj.get("status") not in ("failed", "incomplete", "cancelled", "error"),
            "error_or_incomplete")


def json_value(raw):
    def pairs(items):
        result = {}
        for key, value in items:
            require(key not in result, "duplicate_json_key")
            result[key] = value
        return result

    def constant(_):
        raise CheckFailed("invalid_json_constant")

    try:
        if isinstance(raw, bytes):
            raw = raw.decode("utf-8")
        return json.loads(raw, object_pairs_hook=pairs, parse_constant=constant)
    except (ValueError, UnicodeError, RecursionError):
        raise CheckFailed("invalid_json") from None


def usage_summary(value):
    if value is None:
        return {}
    require(isinstance(value, dict), "invalid_usage")
    result = {}
    for key in USAGE_FIELDS:
        if key in value:
            require(integer(value[key]), "invalid_usage")
            result[key] = value[key]
    # Never copy arbitrary upstream metadata into the report.
    for key in ("input_tokens_details", "output_tokens_details",
                "prompt_tokens_details", "completion_tokens_details"):
        if value.get(key) is not None:
            require(isinstance(value[key], dict), "invalid_usage")
            details = {}
            for field in ("cached_tokens", "reasoning_tokens"):
                if field in value[key]:
                    require(integer(value[key][field]), "invalid_usage")
                    details[field] = value[key][field]
            if details:
                result[key] = details
    return result


def checked_call(name, call_id, arguments, expected_value):
    require(name == TOOL_NAME, "unexpected_tool_name")
    identifier(call_id)
    require(isinstance(arguments, dict) and set(arguments) == {"value"}
            and isinstance(arguments["value"], str)
            and arguments["value"] == expected_value, "invalid_tool_arguments")
    return {"name": name, "id": call_id, "arguments": arguments}


def validate_json(protocol, raw, *, tool=False, expected_value=None):
    doc = json_value(raw)
    no_error(doc)
    model, response_id = identifier(doc.get("model")), identifier(doc.get("id"))
    text, calls = "", []
    if protocol == "chat":
        require(doc.get("object") == "chat.completion", "invalid_response_type")
        choices = doc.get("choices")
        require(isinstance(choices, list) and len(choices) == 1, "invalid_choices")
        choice = choices[0]
        no_error(choice)
        require(type(choice.get("index")) is int and choice["index"] == 0, "invalid_choice_index")
        require(choice.get("finish_reason") == ("tool_calls" if tool else "stop"), "abnormal_finish")
        message = choice.get("message")
        no_error(message)
        require(message.get("role") == "assistant", "invalid_role")
        require(not message.get("refusal") and not message.get("function_call"), "refusal_or_legacy_tool")
        text = message.get("content")
        require(text is None or isinstance(text, str), "invalid_content")
        text = text or ""
        raw_calls = message.get("tool_calls", [])
        require(isinstance(raw_calls, list), "invalid_tool_calls")
        for item in raw_calls:
            no_error(item)
            require(item.get("type") == "function", "unexpected_tool_type")
            function = item.get("function")
            no_error(function)
            require(isinstance(function.get("arguments"), str), "invalid_tool_arguments")
            calls.append(checked_call(function.get("name"), item.get("id"),
                                      json_value(function["arguments"]), expected_value))
    elif protocol == "responses":
        require(doc.get("object") == "response" and doc.get("status") == "completed", "abnormal_finish")
        output = doc.get("output")
        require(isinstance(output, list) and output, "missing_output")
        for item in output:
            no_error(item)
            require(item.get("status", "completed") == "completed", "incomplete_output_item")
            kind = item.get("type")
            if kind == "message":
                require(item.get("role") == "assistant", "invalid_role")
                content = item.get("content")
                require(isinstance(content, list), "invalid_content")
                for part in content:
                    no_error(part)
                    require(part.get("type") == "output_text" and isinstance(part.get("text"), str),
                            "refusal_or_invalid_content")
                    text += part["text"]
            elif kind == "function_call":
                identifier(item.get("id"))  # item id is not the tool-result call_id.
                require(isinstance(item.get("arguments"), str), "invalid_tool_arguments")
                calls.append(checked_call(item.get("name"), item.get("call_id"),
                                          json_value(item["arguments"]), expected_value))
            else:
                require(kind == "reasoning", "unexpected_output_type")
    elif protocol == "messages":
        require(doc.get("type") == "message" and doc.get("role") == "assistant", "invalid_response_type")
        require(doc.get("stop_reason") == ("tool_use" if tool else "end_turn"), "abnormal_finish")
        content = doc.get("content")
        require(isinstance(content, list), "invalid_content")
        for part in content:
            no_error(part)
            kind = part.get("type")
            if kind == "text":
                require(isinstance(part.get("text"), str), "invalid_content")
                text += part["text"]
            elif kind == "tool_use":
                calls.append(checked_call(part.get("name"), part.get("id"), part.get("input"), expected_value))
            else:
                require(kind in ("thinking", "redacted_thinking"), "unexpected_content_type")
    else:
        raise CheckFailed("invalid_protocol")
    if tool:
        require(len(calls) == 1, "expected_one_tool_call")
    else:
        require(not calls and has_text(text), "missing_text_or_unexpected_tool")
    return {"model": model, "responseId": response_id, "usage": usage_summary(doc.get("usage")),
            "eventCount": 0, "contentLength": len(text), "_text": text,
            "_document": doc, "_call": calls[0] if calls else None}


def sse_events(raw):
    try:
        text = raw.decode("utf-8-sig") if isinstance(raw, bytes) else raw
    except UnicodeError:
        raise CheckFailed("invalid_sse_encoding") from None
    require(isinstance(text, str), "invalid_sse")
    text = text.replace("\r\n", "\n").replace("\r", "\n")
    events, data, event = [], [], ""
    # splitlines() hides an unterminated frame; only a blank line dispatches SSE.
    lines = text.split("\n")
    for line in lines[:-1]:
        if not line:
            if data:
                events.append((event, "\n".join(data)))
                require(len(events) <= MAX_EVENTS, "too_many_sse_events")
            else:
                require(not event, "empty_sse_event")
            data, event = [], ""
        elif line.startswith(":"):
            continue
        else:
            field, separator, value = line.partition(":")
            require(bool(separator) and field in ("data", "event", "id", "retry"), "invalid_sse_field")
            value = value[1:] if value.startswith(" ") else value
            if field == "data":
                data.append(value)
            elif field == "event":
                require(not event, "duplicate_sse_event_type")
                event = value
            elif field == "retry":
                require(value.isascii() and value.isdigit(), "invalid_sse_retry")
    require(not lines[-1] and not data and not event, "truncated_sse_frame")
    require(bool(events), "empty_sse")
    return events


def validate_sse(protocol, raw):
    events = sse_events(raw)
    text, model, response_id, usage = "", None, None, {}
    finished, done, started, stop_reason = False, False, False, None
    blocks, closed = {}, set()
    response_parts, part_ids, ended_parts = {}, {}, set()

    def metadata(doc):
        nonlocal model, response_id, usage
        no_error(doc)
        for key, old in (("model", model), ("id", response_id)):
            if key in doc:
                value = identifier(doc[key])
                require(old is None or old == value, "stream_identity_changed")
                if key == "model":
                    model = value
                else:
                    response_id = value
        usage.update(usage_summary(doc.get("usage")))

    for event, data in events:
        if data == "[DONE]":
            require(protocol in ("chat", "responses") and finished and not done, "premature_or_duplicate_done")
            done = True
            continue
        require(not done, "event_after_done")
        obj = json_value(data)
        no_error(obj)
        require(event != "error", "stream_error")
        if protocol == "chat":
            require(event in ("", "message"), "invalid_sse_event_type")
            require(obj.get("object") == "chat.completion.chunk", "invalid_chunk_type")
            metadata(obj)
            choices = obj.get("choices")
            require(isinstance(choices, list), "invalid_choices")
            if not choices:
                require(obj.get("usage") is not None, "empty_chunk")
                continue
            require(len(choices) == 1 and not finished, "chunk_after_finish_or_multiple_choices")
            choice = choices[0]
            no_error(choice)
            require(type(choice.get("index")) is int and choice["index"] == 0, "invalid_choice_index")
            delta = choice.get("delta")
            no_error(delta)
            require(delta.get("role", "assistant") == "assistant", "invalid_role")
            require(not delta.get("tool_calls") and not delta.get("function_call") and not delta.get("refusal"),
                    "unexpected_tool_or_refusal")
            content = delta.get("content")
            require(content is None or isinstance(content, str), "invalid_content")
            text += content or ""
            reason = choice.get("finish_reason")
            require(reason in (None, "stop"), "abnormal_finish")
            finished = reason == "stop"
        elif protocol == "responses":
            require(not finished, "event_after_completion")
            kind = obj.get("type")
            require(isinstance(kind, str) and event in ("", kind), "sse_event_type_mismatch")
            if kind in ("response.created", "response.in_progress"):
                metadata(obj.get("response"))
            elif kind in ("response.output_text.delta", "response.output_text.done"):
                index = (obj.get("output_index"), obj.get("content_index"))
                require(all(integer(i) for i in index), "invalid_output_index")
                item_id = identifier(obj.get("item_id"))
                require(part_ids.setdefault(index, item_id) == item_id, "stream_item_identity_changed")
                require(index not in ended_parts, "text_after_part_done")
                if kind.endswith(".delta"):
                    require(isinstance(obj.get("delta"), str), "invalid_text_delta")
                    response_parts[index] = response_parts.get(index, "") + obj["delta"]
                else:
                    require(index in response_parts and isinstance(obj.get("text"), str)
                            and obj["text"] == response_parts[index], "stream_text_mismatch")
                    ended_parts.add(index)
                text = "".join(response_parts[i] for i in sorted(response_parts))
            elif kind == "response.completed":
                final = validate_json("responses", json.dumps(obj.get("response")))
                metadata(final["_document"])
                require(has_text(text) and text == final["_text"], "stream_text_mismatch")
                output = final["_document"]["output"]
                for (item_index, content_index), item_id in part_ids.items():
                    require(item_index < len(output), "invalid_output_index")
                    item = output[item_index]
                    require(item.get("type") == "message" and item.get("id") == item_id
                            and content_index < len(item["content"])
                            and item["content"][content_index].get("text") == response_parts[(item_index, content_index)],
                            "stream_item_identity_or_text_mismatch")
                finished = True
            elif kind in ("response.output_item.added", "response.output_item.done"):
                item = obj.get("item")
                no_error(item)
                require(item.get("type") in ("message", "reasoning"), "unexpected_output_type")
                if kind.endswith(".done"):
                    require(item.get("status", "completed") == "completed", "incomplete_output_item")
            elif kind in ("response.content_part.added", "response.content_part.done"):
                part = obj.get("part")
                no_error(part)
                require(part.get("type") == "output_text", "unexpected_content_type")
            else:
                require(kind in ("response.reasoning_summary_part.added", "response.reasoning_summary_part.done",
                                 "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done",
                                 "response.reasoning_text.delta", "response.reasoning_text.done"),
                        "unexpected_sse_event")
        elif protocol == "messages":
            require(not finished, "event_after_completion")
            kind = obj.get("type")
            require(isinstance(kind, str) and event in ("", kind), "sse_event_type_mismatch")
            if kind == "ping":
                continue
            if kind == "message_start":
                require(not started, "duplicate_message_start")
                message = obj.get("message")
                metadata(message)
                require(message.get("type") == "message" and message.get("role") == "assistant"
                        and message.get("content") == [] and message.get("stop_reason") is None,
                        "invalid_message_start")
                started = True
            else:
                require(started, "missing_message_start")
                if kind in ("content_block_start", "content_block_delta", "content_block_stop"):
                    index = obj.get("index")
                    require(integer(index) and stop_reason is None, "invalid_block_index_or_order")
                    if kind == "content_block_start":
                        require(index not in blocks, "duplicate_content_block")
                        block = obj.get("content_block")
                        no_error(block)
                        require(block.get("type") in ("text", "thinking", "redacted_thinking"),
                                "unexpected_content_type")
                        initial = block.get("text", "")
                        require(isinstance(initial, str), "invalid_content")
                        blocks[index] = {"type": block["type"], "text": initial if block["type"] == "text" else ""}
                    else:
                        require(index in blocks and index not in closed, "unopened_or_closed_block")
                        if kind == "content_block_stop":
                            closed.add(index)
                        else:
                            delta = obj.get("delta")
                            no_error(delta)
                            if blocks[index]["type"] == "text":
                                require(delta.get("type") == "text_delta" and isinstance(delta.get("text"), str),
                                        "invalid_text_delta")
                                blocks[index]["text"] += delta["text"]
                            else:
                                field = {"thinking_delta": "thinking", "signature_delta": "signature"}.get(delta.get("type"))
                                require(field is not None and isinstance(delta.get(field), str), "invalid_thinking_delta")
                elif kind == "message_delta":
                    delta = obj.get("delta")
                    no_error(delta)
                    require(stop_reason is None and set(blocks) == closed and delta.get("stop_reason") == "end_turn",
                            "abnormal_finish_or_unclosed_block")
                    stop_reason = "end_turn"
                    usage.update(usage_summary(obj.get("usage")))
                elif kind == "message_stop":
                    require(stop_reason == "end_turn" and set(blocks) == closed, "missing_stop_reason_or_unclosed_block")
                    text = "".join(blocks[i]["text"] for i in sorted(blocks))
                    finished = True
                else:
                    raise CheckFailed("unexpected_sse_event")
        else:
            raise CheckFailed("invalid_protocol")
    require(finished and (protocol != "chat" or done), "missing_stream_termination")
    require(has_text(text), "missing_stream_text")
    return {"model": identifier(model), "responseId": identifier(response_id), "usage": usage,
            "eventCount": len(events), "contentLength": len(text), "_text": text}


def validate_http(protocol, reply, *, stream=False, tool=False, expected_value=None):
    status, content_type, body = reply
    require(status == 200, "http_error")
    require(isinstance(body, bytes) and len(body) <= MAX_BODY, "invalid_or_oversized_body")
    require(isinstance(content_type, str), "invalid_content_type")
    mime = content_type.split(";", 1)[0].strip().lower()
    if stream:
        require(mime == "text/event-stream", "expected_sse_content_type")
        return validate_sse(protocol, body)
    require(mime == "application/json" or (mime.startswith("application/") and mime.endswith("+json")),
            "expected_json_content_type")
    return validate_json(protocol, body, tool=tool, expected_value=expected_value)


def request_body(protocol, model, max_tokens, *, stream=False, tool_value=None):
    prompt = "Reply with one short sentence confirming the relay works."
    schema = {"type": "object", "properties": {"value": {"type": "string"}},
              "required": ["value"], "additionalProperties": False}
    function = {"name": TOOL_NAME, "description": "Echo a value and return a receipt; no external actions.",
                "parameters": schema}
    if tool_value is not None:
        prompt = ("Call metapi_echo exactly once with value " + json.dumps(tool_value)
                  + ". After its result, reply with the receipt from that result. Do not call any other tool.")
    message = {"role": "user", "content": prompt}
    body = {"model": model, "stream": stream}
    if protocol == "responses":
        body.update(input=[message], max_output_tokens=max_tokens, store=False)
        if tool_value is not None:
            body.update(tools=[dict(type="function", **function)],
                        tool_choice={"type": "function", "name": TOOL_NAME}, parallel_tool_calls=False)
    else:
        body.update(messages=[message], max_tokens=max_tokens)
        if protocol == "chat":
            body["n"] = 1
            if stream:
                body["stream_options"] = {"include_usage": True}
            if tool_value is not None:
                body.update(tools=[{"type": "function", "function": function}],
                            tool_choice={"type": "function", "function": {"name": TOOL_NAME}},
                            parallel_tool_calls=False)
        elif protocol == "messages":
            if tool_value is not None:
                body.update(tools=[{"name": TOOL_NAME, "description": function["description"], "input_schema": schema}],
                            tool_choice={"type": "tool", "name": TOOL_NAME, "disable_parallel_tool_use": True})
        else:
            raise CheckFailed("invalid_protocol")
    return body


def followup_body(protocol, original, validated, receipt):
    call, doc = validated["_call"], validated["_document"]
    output = json.dumps({"value": call["arguments"]["value"], "receipt": receipt})
    body = dict(original)
    if protocol == "chat":
        # Keep reasoning_content when supplied; thinking-capable models need it on replay.
        body["messages"] = original["messages"] + [doc["choices"][0]["message"],
                                                    {"role": "tool", "tool_call_id": call["id"], "content": output}]
        body["tool_choice"] = "none"
    elif protocol == "responses":
        # Stateless replay, including reasoning items, avoids previous_response_id storage dependencies.
        body["input"] = original["input"] + doc["output"] + [
            {"type": "function_call_output", "call_id": call["id"], "output": output}]
        body["tool_choice"] = "none"
    else:
        body["messages"] = original["messages"] + [
            {"role": "assistant", "content": doc["content"]},
            {"role": "user", "content": [{"type": "tool_result", "tool_use_id": call["id"], "content": output}]}]
        body["tool_choice"] = {"type": "none"}
    return body


def curl_quote(value):
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"').replace("\n", "\\n").replace("\r", "\\r") + '"'


class CurlClient:
    def __init__(self, base_url, key, timeout, environment):
        self.base_url, self.key, self.timeout = base_url, key, timeout
        self.environment = {k: v for k, v in environment.items() if k != "RELAY_API_KEY"}
        self.request_count = 0

    def post(self, protocol, body):
        headers = ["Content-Type: application/json", "Accept: " + ("text/event-stream" if body["stream"] else "application/json")]
        if protocol == "messages":
            headers += ["x-api-key: " + self.key, "anthropic-version: 2023-06-01"]
        else:
            headers += ["Authorization: Bearer " + self.key]
        config = "url = " + curl_quote(self.base_url + PATHS[protocol]) + "\n"
        config += "".join("header = " + curl_quote(h) + "\n" for h in headers)
        config += "data-binary = " + curl_quote(json.dumps(body, ensure_ascii=True)) + "\n"
        # -q MUST be curl's first argument: ignore ~/.curlrc before parsing anything.
        # No redirects, retries, proxy, custom UA, shell, auth argv, or body/header files.
        command = ["curl", "-q", "--noproxy", "*", "--silent", "--no-buffer",
                   "--proto", "=http,https", "--globoff", "--retry", "0", "--max-redirs", "0",
                   "--connect-timeout", str(min(10, self.timeout)), "--max-time", str(self.timeout),
                   "--max-filesize", str(MAX_BODY), "--request", "POST",
                   "--write-out", "\n__METAPI_HTTP__%{http_code}\n%{content_type}", "--config", "-"]
        self.request_count += 1
        try:
            completed = subprocess.run(command, input=config.encode("utf-8"), stdout=subprocess.PIPE,
                                       stderr=subprocess.DEVNULL, timeout=self.timeout + 5, env=self.environment,
                                       check=False)
        except subprocess.TimeoutExpired:
            raise CheckFailed("transport_timeout") from None
        except OSError:
            raise CheckFailed("transport_io_error") from None
        require(completed.returncode == 0, "curl_exit_" + str(completed.returncode))
        require(isinstance(completed.stdout, bytes) and len(completed.stdout) <= MAX_BODY + 4096,
                "invalid_or_oversized_body")
        raw, marker, trailer = completed.stdout.rpartition(HTTP_MARKER)
        require(bool(marker), "missing_http_metadata")
        fields = trailer.split(b"\n")
        require(len(fields) == 2 and len(fields[0]) == 3 and fields[0].isdigit(), "invalid_http_metadata")
        try:
            content_type = fields[1].decode("ascii")
        except UnicodeError:
            raise CheckFailed("invalid_http_metadata") from None
        return int(fields[0]), content_type, raw


def empty_result(protocol, scenario):
    return {"protocol": protocol, "scenario": scenario, "status": "fail", "httpStatus": None,
            "model": None, "requestedModel": None, "responseModel": None, "responseId": None,
            "usage": {}, "eventCount": 0, "contentLength": 0,
            "toolName": None, "followup": None}


def error_code(error):
    # Never format arbitrary exception text: curl/parser/upstream data may include secrets.
    return str(error) if isinstance(error, CheckFailed) else "unexpected_read_or_validation_error"


def observed_response_signals(reply):
    """Bounded, allowlisted diagnostics, never response text or tool arguments."""
    if not isinstance(reply, (tuple, list)) or len(reply) != 3:
        return {}
    try:
        documents = [json_value(reply[2])]
    except CheckFailed:
        try:
            documents = [json_value(data) for _, data in sse_events(reply[2]) if data != "[DONE]"]
        except CheckFailed:
            return {}
    allowed = {"stop", "length", "tool_calls", "function_call", "content_filter",
               "end_turn", "tool_use", "max_tokens", "max_output_tokens", "stop_sequence",
               "pause_turn", "refusal", "model_context_window_exceeded"}
    reasons, tools = [], False

    def note(value):
        if value is not None:
            reasons.append("empty_string" if value == "" else value if isinstance(value, str) and value in allowed else "other")

    for doc in documents:
        if not isinstance(doc, dict):
            continue
        note(doc.get("stop_reason"))
        delta = doc.get("delta")
        if isinstance(delta, dict):
            note(delta.get("stop_reason"))
        choices = doc.get("choices")
        for choice in choices if isinstance(choices, list) else []:
            if not isinstance(choice, dict):
                continue
            note(choice.get("finish_reason"))
            for message in (choice.get("message"), choice.get("delta")):
                if isinstance(message, dict) and isinstance(message.get("tool_calls"), list):
                    tools = tools or bool(message["tool_calls"])
        response = doc.get("response", doc)
        if not isinstance(response, dict):
            continue
        details = response.get("incomplete_details")
        if isinstance(details, dict):
            note(details.get("reason"))
        for name in ("content", "output"):
            parts = response.get(name)
            if isinstance(parts, list):
                tools = tools or any(isinstance(part, dict) and part.get("type") in ("tool_use", "function_call") for part in parts)
    return {"finishReasons": reasons[:20], "finishReasonCount": len(reasons), "toolCallsPresent": tools}


def run_protocol(client, protocol, model, max_tokens):
    results = []
    for scenario in ("nonstream", "stream", "tool_roundtrip"):
        row = empty_result(protocol, scenario)
        row["requestedModel"] = model
        results.append(row)
        tool_value = "echo-" + secrets.token_hex(8) if scenario == "tool_roundtrip" else None
        if tool_value is not None:
            row["followup"] = {"status": "not_run"}
        reply = None
        try:
            body = request_body(protocol, model, max_tokens, stream=scenario == "stream", tool_value=tool_value)
            reply = client.post(protocol, body)
            row["httpStatus"] = reply[0]
            validated = validate_http(protocol, reply, stream=scenario == "stream", tool=tool_value is not None,
                                      expected_value=tool_value)
            row.update({key: validated[key] for key in SUMMARY_FIELDS})
            row["responseModel"] = validated["model"]
            if tool_value is not None:
                row["toolName"] = validated["_call"]["name"]
                followup = empty_result(protocol, "tool_followup")
                followup["requestedModel"] = model
                followup.pop("followup")
                row["followup"] = followup
                # This receipt exists only in the tool result, not in the first prompt.
                receipt = "receipt-" + secrets.token_hex(12)
                followup_reply = None
                try:
                    followup_reply = client.post(protocol, followup_body(protocol, body, validated, receipt))
                    followup["httpStatus"] = followup_reply[0]
                    final = validate_http(protocol, followup_reply)
                    followup.update({key: final[key] for key in SUMMARY_FIELDS})
                    followup["responseModel"] = final["model"]
                    require(receipt in final["_text"], "followup_did_not_use_tool_result")
                    followup["status"] = "pass"
                except Exception as error:
                    followup["error"] = error_code(error)
                    followup["observed"] = observed_response_signals(followup_reply)
                    raise CheckFailed("tool_followup_failed") from None
            row["status"] = "pass"
        except Exception as error:
            row["error"] = error_code(error)
            row["observed"] = observed_response_signals(reply)
    return results


class Parser(argparse.ArgumentParser):
    def error(self, message):
        raise CheckFailed("invalid_arguments")


def main(argv=None, environment=None):
    environment = os.environ if environment is None else environment
    key = environment.get("RELAY_API_KEY", "")
    report = {"kind": "relay_acceptance", "evidence": "http_observations",
              "upstreamProvenance": "not_verified", "status": "fail", "requestCount": 0, "results": []}
    exit_code = 2
    try:
        parser = Parser(description=__doc__)
        parser.add_argument("--protocol", choices=("all",) + PROTOCOLS, default="all")
        parser.add_argument("--timeout", type=int, default=120, metavar="SECONDS", help="per POST, 5..300 (default: 120)")
        parser.add_argument("--max-tokens", type=int, default=256, metavar="TOKENS", help="per POST, 64..4096 (default: 256)")
        args = parser.parse_args(argv)
        require(5 <= args.timeout <= 300 and 64 <= args.max_tokens <= 4096, "invalid_limits")
        report["maxTokensPerRequest"] = args.max_tokens
        report["perRequestTimeoutSeconds"] = args.timeout
        base, model = environment.get("RELAY_BASE_URL", ""), environment.get("RELAY_MODEL", "")
        require(bool(base and key and model), "missing_relay_environment")
        require(len(key) <= 4096 and all(33 <= ord(c) <= 126 for c in key), "invalid_api_key_format")
        identifier(model)
        require(all(c.isprintable() and not c.isspace() for c in base), "invalid_base_url")
        url = urlsplit(base)
        require(url.scheme in ("http", "https") and bool(url.hostname) and not url.username
                and not url.password and not url.query and not url.fragment, "invalid_base_url")
        _ = url.port  # Invalid ports must fail before any model request.
        base = base.rstrip("/")
        if not base.endswith("/v1"):
            base += "/v1"
        selected = PROTOCOLS if args.protocol == "all" else (args.protocol,)
        report["requestLimit"] = len(selected) * 4
        client = CurlClient(base, key, args.timeout, environment)
        for protocol in selected:
            report["results"].extend(run_protocol(client, protocol, model, args.max_tokens))
        report["requestCount"] = client.request_count
        passed = bool(report["results"]) and all(row["status"] == "pass" for row in report["results"])
        report["status"] = "pass" if passed else "fail"
        exit_code = 0 if passed else 1
    except Exception as error:
        report["error"] = error_code(error)
    # Upstream-controlled identifiers must not echo even a deliberately reflected key.
    def redact(value):
        if isinstance(value, dict):
            return {k: redact(v) for k, v in value.items()}
        if isinstance(value, list):
            return [redact(v) for v in value]
        return value.replace(key, "[REDACTED]") if key and isinstance(value, str) else value

    print(json.dumps(redact(report), ensure_ascii=True, separators=(",", ":")))
    return exit_code


if __name__ == "__main__":
    sys.exit(main())
