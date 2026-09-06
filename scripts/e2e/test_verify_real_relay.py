#!/usr/bin/env python3
"""Offline validator tests. Fixtures/mocked curl are NOT live relay proof.

Run: PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts/e2e \
     -p test_verify_real_relay.py -v
No credentials, sockets, providers, services, or installed dependencies are used.
"""

import contextlib
import copy
import importlib.util
import io
import json
import subprocess
import unittest
from pathlib import Path
from unittest import mock

SPEC = importlib.util.spec_from_file_location("verify_real_relay", Path(__file__).with_name("verify-real-relay.py"))
relay = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(relay)

MODEL = "fixture-response-model"
VALUE = "echo-fixture"
RECEIPT = "receipt-fixture"
TEXT = "Private fixture text, not report output."
KEY = "fixture-key-never-log"
ENV = {"RELAY_BASE_URL": "http://127.0.0.1:12345", "RELAY_API_KEY": KEY,
       "RELAY_MODEL": "fixture-request-model", "PATH": "/usr/bin:/bin"}


def encode(value):
    return json.dumps(value).encode("utf-8")


def document(protocol, text=TEXT, tool=False):
    if protocol == "chat":
        message = {"role": "assistant", "content": None if tool else text}
        if tool:
            message.update(reasoning_content="Preserve this reasoning on replay.", tool_calls=[{
                "id": "call_fixture", "type": "function", "function": {
                    "name": "metapi_echo", "arguments": json.dumps({"value": VALUE})}}])
        return {"id": "chat_fixture", "model": MODEL, "object": "chat.completion",
                "choices": [{"index": 0, "message": message, "finish_reason": "tool_calls" if tool else "stop"}],
                "usage": {"prompt_tokens": 12, "completion_tokens": 5, "total_tokens": 17}}
    if protocol == "responses":
        output = [{"id": "msg_fixture", "type": "message", "role": "assistant", "status": "completed",
                   "content": [{"type": "output_text", "text": text, "annotations": []}]}]
        if tool:
            output = [{"id": "rs_fixture", "type": "reasoning", "summary": []},
                      {"id": "fc_fixture", "call_id": "call_fixture", "type": "function_call",
                       "status": "completed", "name": "metapi_echo", "arguments": json.dumps({"value": VALUE})}]
        return {"id": "resp_fixture", "model": MODEL, "object": "response", "status": "completed",
                "error": None, "incomplete_details": None, "output": output,
                "usage": {"input_tokens": 12, "output_tokens": 5, "total_tokens": 17}}
    content = [{"type": "text", "text": text}]
    if tool:
        content = [{"type": "tool_use", "name": "metapi_echo", "id": "toolu_fixture", "input": {"value": VALUE}}]
    return {"id": "msg_fixture", "model": MODEL, "type": "message", "role": "assistant",
            "content": content, "stop_reason": "tool_use" if tool else "end_turn", "stop_sequence": None,
            "usage": {"input_tokens": 12, "output_tokens": 5}}


def call_item(protocol, doc):
    if protocol == "chat":
        return doc["choices"][0]["message"]["tool_calls"][0]
    return doc["output"][-1] if protocol == "responses" else doc["content"][0]


def stream_events(protocol):
    doc = document(protocol)
    if protocol == "chat":
        def chunk(delta, finish=None):
            return {"id": doc["id"], "model": MODEL, "object": "chat.completion.chunk",
                    "choices": [{"index": 0, "delta": delta, "finish_reason": finish}]}
        return [("", chunk({"role": "assistant", "content": ""})),
                ("", chunk({"content": TEXT})), ("", chunk({}, "stop")),
                ("", {"id": doc["id"], "model": MODEL, "object": "chat.completion.chunk",
                      "choices": [], "usage": doc["usage"]}), ("", "[DONE]")]
    if protocol == "responses":
        return [("response.created", {"type": "response.created", "response": {
                    "id": doc["id"], "model": MODEL, "status": "in_progress"}}),
                ("response.output_text.delta", {"type": "response.output_text.delta", "item_id": "msg_fixture",
                    "output_index": 0, "content_index": 0, "delta": TEXT}),
                ("response.output_text.done", {"type": "response.output_text.done", "item_id": "msg_fixture",
                    "output_index": 0, "content_index": 0, "text": TEXT}),
                ("response.completed", {"type": "response.completed", "response": doc})]
    start = dict(doc, content=[], stop_reason=None, usage={"input_tokens": 12})
    return [("message_start", {"type": "message_start", "message": start}),
            ("content_block_start", {"type": "content_block_start", "index": 0,
                                     "content_block": {"type": "text", "text": ""}}),
            ("content_block_delta", {"type": "content_block_delta", "index": 0,
                                     "delta": {"type": "text_delta", "text": TEXT}}),
            ("content_block_stop", {"type": "content_block_stop", "index": 0}),
            ("message_delta", {"type": "message_delta", "delta": {"stop_reason": "end_turn"},
                               "usage": {"output_tokens": 5}}),
            ("message_stop", {"type": "message_stop"})]


def pack(events):
    return "".join(("event: " + event + "\n" if event else "") + "data: "
                   + (data if isinstance(data, str) else json.dumps(data)) + "\n\n"
                   for event, data in events).encode("utf-8")


class ValidatorTests(unittest.TestCase):
    def test_all_normal_json_fixtures(self):
        for protocol in relay.PROTOCOLS:
            with self.subTest(protocol=protocol):
                value = relay.validate_http(protocol, (200, "application/json; charset=utf-8", encode(document(protocol))))
                self.assertEqual(value["_text"], TEXT)
                self.assertEqual(value["model"], MODEL)
                self.assertEqual(value["contentLength"], len(TEXT))
                self.assertGreater(value["usage"].get("output_tokens", value["usage"].get("completion_tokens", 0)), 0)

    def test_all_normal_sse_fixtures(self):
        for protocol in relay.PROTOCOLS:
            with self.subTest(protocol=protocol):
                events = stream_events(protocol)
                value = relay.validate_http(protocol, (200, "text/event-stream; charset=utf-8", pack(events)), stream=True)
                self.assertEqual(value["_text"], TEXT)
                self.assertEqual(value["eventCount"], len(events))
                self.assertEqual(value["responseId"], document(protocol)["id"])
                self.assertTrue(value["usage"])

    def test_all_normal_tool_fixtures(self):
        for protocol in relay.PROTOCOLS:
            with self.subTest(protocol=protocol):
                value = relay.validate_json(protocol, encode(document(protocol, tool=True)), tool=True, expected_value=VALUE)
                self.assertEqual(value["_call"]["name"], "metapi_echo")
                self.assertEqual(value["_call"]["arguments"], {"value": VALUE})

    def test_damaged_json_and_non_objects(self):
        for protocol in relay.PROTOCOLS:
            for raw in (b'{"broken":', b'{} trailing', b'[]', b'null', b'"text"', b'\xff',
                        b'{"x":NaN}', b'{"x":Infinity}', b'{"x":1,"x":2}'):
                with self.subTest(protocol=protocol, raw=raw):
                    with self.assertRaises(relay.CheckFailed):
                        relay.validate_json(protocol, raw)

    def test_http_200_errors_never_pass_even_with_valid_content(self):
        for protocol in relay.PROTOCOLS:
            for error in ({"message": "not success"}, {}, "upstream failed"):
                doc = document(protocol)
                doc["error"] = error
                with self.subTest(protocol=protocol, error=error):
                    with self.assertRaises(relay.CheckFailed):
                        relay.validate_http(protocol, (200, "application/json", encode(doc)))

    def test_non_200_and_wrong_mime_fail(self):
        for protocol in relay.PROTOCOLS:
            for status, mime in ((401, "application/json"), (500, "application/json"), (302, "application/json"),
                                 (200, "text/html"), (200, "")):
                with self.subTest(protocol=protocol, status=status, mime=mime):
                    with self.assertRaises(relay.CheckFailed):
                        relay.validate_http(protocol, (status, mime, encode(document(protocol))))
            with self.assertRaises(relay.CheckFailed):
                relay.validate_http(protocol, (200, "application/json", pack(stream_events(protocol))), stream=True)

    def test_truncated_or_abnormal_json_finish(self):
        for protocol in relay.PROTOCOLS:
            for reason in (None, "length", "max_tokens", "content_filter", "incomplete", "failed"):
                doc = document(protocol)
                if protocol == "chat":
                    doc["choices"][0]["finish_reason"] = reason
                else:
                    doc["status" if protocol == "responses" else "stop_reason"] = reason
                with self.subTest(protocol=protocol, reason=reason):
                    with self.assertRaises(relay.CheckFailed):
                        relay.validate_json(protocol, encode(doc))
        doc = document("responses")
        doc["incomplete_details"] = {"reason": "max_output_tokens"}
        with self.assertRaises(relay.CheckFailed):
            relay.validate_json("responses", encode(doc))

    def test_empty_or_reasoning_only_is_not_content(self):
        for protocol in relay.PROTOCOLS:
            for text in ("", "  \n", "\x00", "\ud800"):
                with self.subTest(protocol=protocol, text=text):
                    with self.assertRaises(relay.CheckFailed):
                        relay.validate_json(protocol, encode(document(protocol, text=text)))
            events = stream_events(protocol)
            raw = pack(events).replace(TEXT.encode(), b"")
            with self.assertRaises(relay.CheckFailed):
                relay.validate_sse(protocol, raw)
        doc = document("responses")
        doc["output"] = [{"type": "reasoning", "summary": [{"type": "summary_text", "text": TEXT}]}]
        with self.assertRaises(relay.CheckFailed):
            relay.validate_json("responses", encode(doc))

    def test_bad_usage_fails_instead_of_hiding_read_error(self):
        for protocol in relay.PROTOCOLS:
            for usage in ([], "unknown", {"output_tokens": -1}, {"input_tokens": True},
                          {"output_tokens_details": "not an object"}):
                doc = document(protocol)
                doc["usage"] = usage
                with self.subTest(protocol=protocol, usage=usage):
                    with self.assertRaises(relay.CheckFailed):
                        relay.validate_json(protocol, encode(doc))
        self.assertEqual(relay.usage_summary(None), {})
        self.assertNotIn("payload", relay.usage_summary({"input_tokens": 1, "payload": TEXT}))

    def test_missing_terminal_or_terminal_frame_is_failure(self):
        for protocol in relay.PROTOCOLS:
            raw = pack(stream_events(protocol))
            for broken in (pack(stream_events(protocol)[:-1]), raw.rstrip(b"\n"), raw[:-1]):
                with self.subTest(protocol=protocol, tail=broken[-35:]):
                    with self.assertRaises(relay.CheckFailed):
                        relay.validate_sse(protocol, broken)

    def test_malformed_sse_never_accepts_a_valid_prefix(self):
        for protocol in relay.PROTOCOLS:
            for tail in (b'data: {oops}\n\n', b'data: {"broken":', b'not-sse\n\n', b'\xff\n\n'):
                with self.subTest(protocol=protocol, tail=tail):
                    with self.assertRaises(relay.CheckFailed):
                        relay.validate_sse(protocol, pack(stream_events(protocol)) + tail)
            with self.assertRaises(relay.CheckFailed):
                relay.validate_sse(protocol, b': heartbeat\n\n')

    def test_http_200_sse_error_before_or_after_terminal_fails(self):
        for protocol in relay.PROTOCOLS:
            events = stream_events(protocol)
            error = ("error", {"type": "error", "error": {"message": "private failure detail"}})
            for damaged in ([error] + events, events + [error], events[:-1] + [error]):
                with self.subTest(protocol=protocol, length=len(damaged)):
                    with self.assertRaises(relay.CheckFailed):
                        relay.validate_http(protocol, (200, "text/event-stream", pack(damaged)), stream=True)

    def test_each_stream_abnormal_finish_fails(self):
        for protocol in relay.PROTOCOLS:
            events = stream_events(protocol)
            if protocol == "chat":
                events[2][1]["choices"][0]["finish_reason"] = "length"
            elif protocol == "responses":
                events[-1][1]["response"]["status"] = "incomplete"
            else:
                events[-2][1]["delta"]["stop_reason"] = "max_tokens"
            with self.subTest(protocol=protocol):
                with self.assertRaises(relay.CheckFailed):
                    relay.validate_sse(protocol, pack(events))

    def test_chat_needs_finish_reason_and_done(self):
        events = stream_events("chat")
        del events[2]
        with self.assertRaises(relay.CheckFailed):
            relay.validate_sse("chat", pack(events))
        events = stream_events("chat")
        events[1][1]["id"] = "wrong_response_id"
        with self.assertRaises(relay.CheckFailed):
            relay.validate_sse("chat", pack(events))
        with self.assertRaises(relay.CheckFailed):
            relay.validate_sse("chat", pack(stream_events("chat") + [("", "[DONE]")]))

    def test_responses_completion_snapshot_must_match_stream(self):
        for field, value in (("id", "wrong_response"), ("model", "wrong_stream_model")):
            events = stream_events("responses")
            events[-1][1]["response"][field] = value
            with self.assertRaises(relay.CheckFailed):
                relay.validate_sse("responses", pack(events))
        for field, value in (("item_id", "wrong_item"), ("output_index", 3), ("content_index", 2)):
            events = stream_events("responses")
            events[1][1][field] = value
            with self.assertRaises(relay.CheckFailed):
                relay.validate_sse("responses", pack(events))
        events = stream_events("responses")
        events[-1][1]["response"]["output"][0]["content"][0]["text"] = "different text"
        with self.assertRaises(relay.CheckFailed):
            relay.validate_sse("responses", pack(events))
        self.assertTrue(relay.validate_sse("responses", pack(stream_events("responses") + [("", "[DONE]")])))

    def test_responses_failed_or_incomplete_event_and_type_mismatch(self):
        for kind in ("response.failed", "response.incomplete"):
            events = stream_events("responses")[:-1] + [(kind, {"type": kind, "response": document("responses")})]
            with self.assertRaises(relay.CheckFailed):
                relay.validate_sse("responses", pack(events))
        events = stream_events("responses")
        events[1] = ("response.completed", events[1][1])
        with self.assertRaises(relay.CheckFailed):
            relay.validate_sse("responses", pack(events))

    def test_anthropic_requires_full_block_and_message_lifecycle(self):
        for missing_index in (0, 1, 3, 4, 5):
            events = stream_events("messages")
            del events[missing_index]
            with self.subTest(missing_index=missing_index):
                with self.assertRaises(relay.CheckFailed):
                    relay.validate_sse("messages", pack(events))
        events = stream_events("messages")
        events[2][1]["index"] = 99
        with self.assertRaises(relay.CheckFailed):
            relay.validate_sse("messages", pack(events))

    def test_sse_crlf_comments_and_multiline_json(self):
        for protocol in relay.PROTOCOLS:
            raw = b': heartbeat\n\n' + pack(stream_events(protocol))
            raw = raw.replace(b'"model": ', b'"model":\ndata: ', 1).replace(b'\n', b'\r\n')
            with self.subTest(protocol=protocol):
                self.assertTrue(relay.validate_sse(protocol, raw))

    def test_missing_toolcall_is_failure(self):
        for protocol in relay.PROTOCOLS:
            with self.subTest(protocol=protocol):
                with self.assertRaises(relay.CheckFailed):
                    relay.validate_json(protocol, encode(document(protocol)), tool=True, expected_value=VALUE)

    def test_wrong_tool_name_and_builtin_tools_fail(self):
        for protocol in relay.PROTOCOLS:
            for name in ("shell", "web_search", "not_metapi_echo", None):
                doc = document(protocol, tool=True)
                item = call_item(protocol, doc)
                (item["function"] if protocol == "chat" else item)["name"] = name
                with self.subTest(protocol=protocol, name=name):
                    with self.assertRaises(relay.CheckFailed):
                        relay.validate_json(protocol, encode(doc), tool=True, expected_value=VALUE)

    def test_missing_or_invalid_tool_ids_fail(self):
        for protocol in relay.PROTOCOLS:
            fields = ("id", "call_id") if protocol == "responses" else ("id",)
            for field in fields:
                for bad_id in (None, "", "has spaces", "line\nbreak", [], 42):
                    doc = document(protocol, tool=True)
                    call_item(protocol, doc)[field] = bad_id
                    with self.subTest(protocol=protocol, field=field, bad_id=bad_id):
                        with self.assertRaises(relay.CheckFailed):
                            relay.validate_json(protocol, encode(doc), tool=True, expected_value=VALUE)

    def test_unparseable_wrong_and_ambiguous_tool_args_fail(self):
        for protocol in relay.PROTOCOLS:
            invalid = [None, [], {}, {"value": 42}, {"value": "wrong"}, {"value": VALUE, "extra": "no"}]
            for args in invalid:
                doc = document(protocol, tool=True)
                item = call_item(protocol, doc)
                if protocol == "chat":
                    item["function"]["arguments"] = json.dumps(args)
                elif protocol == "responses":
                    item["arguments"] = json.dumps(args)
                else:
                    item["input"] = args
                with self.subTest(protocol=protocol, args=args):
                    with self.assertRaises(relay.CheckFailed):
                        relay.validate_json(protocol, encode(doc), tool=True, expected_value=VALUE)
            if protocol != "messages":
                for raw in ('{"value":', '{"value":"wrong","value":"echo-fixture"}', 'NaN'):
                    doc = document(protocol, tool=True)
                    item = call_item(protocol, doc)
                    (item["function"] if protocol == "chat" else item)["arguments"] = raw
                    with self.assertRaises(relay.CheckFailed):
                        relay.validate_json(protocol, encode(doc), tool=True, expected_value=VALUE)

    def test_multiple_calls_and_tool_in_text_scenario_fail(self):
        for protocol in relay.PROTOCOLS:
            doc = document(protocol, tool=True)
            with self.assertRaises(relay.CheckFailed):
                relay.validate_json(protocol, encode(doc))
            if protocol == "chat":
                doc["choices"][0]["message"]["tool_calls"] *= 2
            elif protocol == "responses":
                doc["output"].append(copy.deepcopy(doc["output"][-1]))
            else:
                doc["content"] *= 2
            with self.assertRaises(relay.CheckFailed):
                relay.validate_json(protocol, encode(doc), tool=True, expected_value=VALUE)

    def test_followup_preserves_call_ids_reasoning_and_exact_echo_result(self):
        for protocol in relay.PROTOCOLS:
            original = relay.request_body(protocol, "requested", 256, tool_value=VALUE)
            before = copy.deepcopy(original)
            doc = document(protocol, tool=True)
            validated = relay.validate_json(protocol, encode(doc), tool=True, expected_value=VALUE)
            followup = relay.followup_body(protocol, original, validated, RECEIPT)
            with self.subTest(protocol=protocol):
                self.assertEqual(original, before)
                if protocol == "chat":
                    result = followup["messages"][-1]
                    self.assertEqual(result["tool_call_id"], "call_fixture")
                    self.assertEqual(followup["messages"][-2], doc["choices"][0]["message"])
                    raw = result["content"]
                elif protocol == "responses":
                    self.assertNotIn("previous_response_id", followup)
                    self.assertFalse(followup["store"])
                    self.assertEqual(followup["input"][-1]["call_id"], "call_fixture")
                    self.assertNotEqual(followup["input"][-1]["call_id"], "fc_fixture")
                    self.assertEqual(followup["input"][1:-1], doc["output"])
                    raw = followup["input"][-1]["output"]
                else:
                    self.assertEqual(followup["messages"][-2]["content"], doc["content"])
                    result = followup["messages"][-1]["content"][0]
                    self.assertEqual(result["tool_use_id"], "toolu_fixture")
                    raw = result["content"]
                self.assertEqual(json.loads(raw), {"value": VALUE, "receipt": RECEIPT})
                self.assertEqual(followup["tool_choice"], {"type": "none"} if protocol == "messages" else "none")
                self.assertEqual(len(followup["tools"]), 1)


    def test_responses_multiple_text_parts(self):
        doc = document("responses", text="First ")
        doc["output"][0]["content"].append({"type": "output_text", "text": "second."})
        events = stream_events("responses")[:1]
        for index, text in enumerate(("First ", "second.")):
            base = {"item_id": "msg_fixture", "output_index": 0, "content_index": index}
            events += [("response.output_text.delta", dict(base, type="response.output_text.delta", delta=text)),
                       ("response.output_text.done", dict(base, type="response.output_text.done", text=text))]
        events.append(("response.completed", {"type": "response.completed", "response": doc}))
        self.assertEqual(relay.validate_sse("responses", pack(events))["_text"], "First second.")

    def test_reasoning_streams_can_precede_text_but_do_not_count_as_content(self):
        events = stream_events("chat")
        reasoning = copy.deepcopy(events[1][1])
        reasoning["choices"][0]["delta"] = {"reasoning_content": "not visible text"}
        events.insert(1, ("", reasoning))
        self.assertEqual(relay.validate_sse("chat", pack(events))["contentLength"], len(TEXT))
        events = stream_events("responses")
        events.insert(1, ("response.reasoning_summary_text.delta", {
            "type": "response.reasoning_summary_text.delta", "delta": "not visible text"}))
        for _, obj in events:
            if "output_index" in obj:
                obj["output_index"] = 1
        events[-1][1]["response"]["output"].insert(0, {"type": "reasoning", "id": "rs_fixture", "summary": []})
        self.assertEqual(relay.validate_sse("responses", pack(events))["contentLength"], len(TEXT))
        events = stream_events("messages")
        for _, obj in events:
            if "index" in obj:
                obj["index"] = 1
        reasoning = [("content_block_start", {"type": "content_block_start", "index": 0,
                                              "content_block": {"type": "thinking", "thinking": ""}}),
                     ("content_block_delta", {"type": "content_block_delta", "index": 0,
                                              "delta": {"type": "thinking_delta", "thinking": "not visible text"}}),
                     ("content_block_delta", {"type": "content_block_delta", "index": 0,
                                              "delta": {"type": "signature_delta", "signature": "fixture-signature"}}),
                     ("content_block_stop", {"type": "content_block_stop", "index": 0})]
        events[1:1] = reasoning
        self.assertEqual(relay.validate_sse("messages", pack(events))["contentLength"], len(TEXT))

    def test_body_and_sse_event_caps_fail_closed(self):
        for protocol in relay.PROTOCOLS:
            with self.subTest(protocol=protocol):
                with mock.patch.object(relay, "MAX_BODY", 16):
                    with self.assertRaises(relay.CheckFailed):
                        relay.validate_http(protocol, (200, "application/json", encode(document(protocol))))
                with mock.patch.object(relay, "MAX_EVENTS", 1):
                    with self.assertRaises(relay.CheckFailed):
                        relay.validate_sse(protocol, pack(stream_events(protocol)))


class TransportAndCliValidatorTests(unittest.TestCase):
    def client(self):
        return relay.CurlClient("http://127.0.0.1:12345/v1", KEY, 10, dict(ENV))

    def test_curl_auth_only_in_stdin_config_and_no_hidden_config_proxy_or_retry(self):
        raw = encode(document("chat")) + relay.HTTP_MARKER + b'200\napplication/json'
        for protocol in relay.PROTOCOLS:
            with self.subTest(protocol=protocol):
                with mock.patch.object(relay.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, raw)) as run:
                    self.client().post(protocol, relay.request_body(protocol, "requested", 256))
                args, kwargs = run.call_args
                argv = args[0]
                self.assertEqual(argv[:4], ["curl", "-q", "--noproxy", "*"])
                self.assertEqual(argv[-2:], ["--config", "-"])
                self.assertEqual(argv[argv.index("--retry") + 1], "0")
                self.assertNotIn(KEY, repr(argv))
                self.assertNotIn("RELAY_API_KEY", kwargs["env"])
                self.assertIn(KEY, kwargs["input"].decode())
                self.assertNotIn("--user-agent", argv)
                self.assertNotIn("--location", argv)
                self.assertIs(kwargs["stderr"], subprocess.DEVNULL)
                self.assertLessEqual(kwargs["timeout"], 15)
                if protocol == "messages":
                    self.assertIn(b'anthropic-version: 2023-06-01', kwargs["input"])
                    self.assertIn(b'x-api-key: ', kwargs["input"])
                else:
                    self.assertIn(b'Authorization: Bearer ', kwargs["input"])

    def test_curl_quote_cannot_inject_a_config_line(self):
        value = 'header "value"\\backslash\nnext\rline'
        quoted = relay.curl_quote(value)
        self.assertNotIn('\n', quoted)
        self.assertNotIn('\r', quoted)
        self.assertIn('\\"value\\"', quoted)
        self.assertIn('\\\\backslash', quoted)

    def test_transport_failure_timeout_and_bad_metadata_never_accept_valid_prefix(self):
        body = encode(document("chat"))
        cases = [subprocess.CompletedProcess([], 28, body + relay.HTTP_MARKER + b'200\napplication/json'),
                 subprocess.CompletedProcess([], 0, body),
                 subprocess.CompletedProcess([], 0, body + relay.HTTP_MARKER + b'bad\napplication/json'),
                 subprocess.CompletedProcess([], 0, body + relay.HTTP_MARKER + b'200\n\xff'),
                 subprocess.CompletedProcess([], 0, b'x' * (relay.MAX_BODY + 4097)),
                 subprocess.TimeoutExpired(["curl"], 10, output=KEY.encode()), OSError(KEY)]
        for outcome in cases:
            with self.subTest(outcome=type(outcome).__name__):
                kwargs = {"side_effect": outcome} if isinstance(outcome, Exception) else {"return_value": outcome}
                with mock.patch.object(relay.subprocess, "run", **kwargs) as run:
                    with self.assertRaises(relay.CheckFailed):
                        self.client().post("chat", relay.request_body("chat", "requested", 256))
                    self.assertEqual(run.call_count, 1)

    def run_cli(self, argv, post, env=None):
        stdout, stderr = io.StringIO(), io.StringIO()
        with mock.patch.object(relay.CurlClient, "post", post), mock.patch.object(relay.secrets, "token_hex", return_value="fixture"):
            with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
                code = relay.main(argv, dict(ENV if env is None else env))
        self.assertEqual(stderr.getvalue(), "")
        return code, json.loads(stdout.getvalue()), stdout.getvalue()

    @staticmethod
    def good_post(client, protocol, body):
        client.request_count += 1
        if body["stream"]:
            return 200, "text/event-stream", pack(stream_events(protocol))
        if body.get("tool_choice") in ("none", {"type": "none"}):
            doc = document(protocol, text=RECEIPT)
        else:
            doc = document(protocol, tool="tools" in body)
        return 200, "application/json", encode(doc)

    def test_all_protocols_use_twelve_posts_and_do_not_require_model_name_equality(self):
        code, report, raw = self.run_cli([], self.good_post)
        self.assertEqual(code, 0)
        self.assertEqual(report["status"], "pass")
        self.assertEqual(report["requestCount"], 12)
        self.assertEqual(report["requestLimit"], 12)
        self.assertEqual(len(report["results"]), 9)
        self.assertEqual(report["upstreamProvenance"], "not_verified")
        for row in report["results"]:
            self.assertEqual(row["requestedModel"], ENV["RELAY_MODEL"])
            self.assertEqual(row["responseModel"], MODEL)
            self.assertNotEqual(row["requestedModel"], row["responseModel"])
            self.assertEqual(row["status"], "pass")
            if row["scenario"] == "tool_roundtrip":
                self.assertEqual(row["followup"]["status"], "pass")
        for private in (KEY, TEXT, VALUE, RECEIPT, "Preserve this reasoning"):
            self.assertNotIn(private, raw)

    def test_failed_responses_report_bounded_terminal_signals_not_payloads(self):
        def limited(client, protocol, body):
            client.request_count += 1
            doc = document(protocol, tool="tools" in body)
            doc["choices"][0]["finish_reason"] = "length"
            return 200, "application/json", encode(doc)
        code, report, raw = self.run_cli(["--protocol", "chat"], limited)
        self.assertEqual(code, 1)
        self.assertEqual(report["maxTokensPerRequest"], 256)
        self.assertEqual(report["results"][0]["observed"]["finishReasons"], ["length"])
        self.assertTrue(report["results"][2]["observed"]["toolCallsPresent"])
        for private in (KEY, TEXT, VALUE, "Preserve this reasoning"):
            self.assertNotIn(private, raw)

    def test_terminal_diagnostics_preserve_duplicates_and_redact_unknown_reasons(self):
        frame = encode({"choices": [{"finish_reason": "stop"}]}).decode()
        raw = ("data: " + frame + "\n\n") * 25 + "data: [DONE]\n\n"
        signals = relay.observed_response_signals((200, "text/event-stream", raw.encode()))
        self.assertEqual(signals["finishReasonCount"], 25)
        self.assertEqual(signals["finishReasons"], ["stop"] * 20)
        signals = relay.observed_response_signals((200, "application/json", encode({"stop_reason": KEY, "content": [{"type": "tool_use", "input": {"value": TEXT}}]})))
        self.assertEqual(signals["finishReasons"], ["other"])
        self.assertTrue(signals["toolCallsPresent"])
        self.assertNotIn(KEY, json.dumps(signals))
        self.assertNotIn(TEXT, json.dumps(signals))
        empty = relay.observed_response_signals((200, "application/json", encode({"choices": [{"finish_reason": ""}]})))
        self.assertEqual(empty["finishReasons"], ["empty_string"])

    def test_protocol_selection_uses_exactly_four_posts(self):
        for protocol in relay.PROTOCOLS:
            with self.subTest(protocol=protocol):
                code, report, _ = self.run_cli(["--protocol", protocol], self.good_post)
                self.assertEqual(code, 0)
                self.assertEqual(report["requestCount"], 4)
                self.assertEqual({r["protocol"] for r in report["results"]}, {protocol})

    def test_missing_tool_skips_followup_and_is_nonzero(self):
        def post(client, protocol, body):
            if "tools" in body:
                client.request_count += 1
                return 200, "application/json", encode(document(protocol))
            return self.good_post(client, protocol, body)
        code, report, _ = self.run_cli([], post)
        self.assertEqual(code, 1)
        self.assertEqual(report["requestCount"], 9)
        for row in report["results"]:
            if row["scenario"] == "tool_roundtrip":
                self.assertEqual(row["status"], "fail")
                self.assertEqual(row["followup"]["status"], "not_run")

    def test_followup_requires_receipt_not_just_any_text(self):
        def post(client, protocol, body):
            if body.get("tool_choice") in ("none", {"type": "none"}):
                client.request_count += 1
                return 200, "application/json", encode(document(protocol))
            return self.good_post(client, protocol, body)
        code, report, _ = self.run_cli([], post)
        self.assertEqual(code, 1)
        self.assertEqual(report["requestCount"], 12)
        for row in report["results"]:
            if row["scenario"] == "tool_roundtrip":
                self.assertEqual(row["followup"]["error"], "followup_did_not_use_tool_result")

    def test_arbitrary_read_exception_fails_without_dumping_exception_data(self):
        def post(client, protocol, body):
            client.request_count += 1
            raise RuntimeError(KEY + TEXT)
        code, report, raw = self.run_cli(["--protocol", "chat"], post)
        self.assertEqual(code, 1)
        self.assertEqual(report["requestCount"], 3)
        self.assertTrue(all(row["status"] == "fail" for row in report["results"]))
        self.assertNotIn(KEY, raw)
        self.assertNotIn(TEXT, raw)

    def test_reflected_key_is_redacted_in_metadata(self):
        def post(client, protocol, body):
            status, mime, raw = self.good_post(client, protocol, body)
            return status, mime, raw.replace(MODEL.encode(), KEY.encode())
        code, report, raw = self.run_cli(["--protocol", "chat"], post)
        self.assertEqual(code, 0)
        self.assertNotIn(KEY, raw)
        self.assertEqual(report["results"][0]["responseModel"], "[REDACTED]")

    def test_invalid_environment_or_limits_make_no_requests(self):
        cases = [([], {}), (["--timeout", "0"], ENV), (["--max-tokens", "999999"], ENV),
                 (["--protocol", "wrong"], ENV), ([], dict(ENV, RELAY_BASE_URL="http://user:password@example.invalid")),
                 ([], dict(ENV, RELAY_API_KEY="bad\nheader")), ([], dict(ENV, RELAY_BASE_URL="http://host:bad"))]
        for argv, env in cases:
            with self.subTest(argv=argv, keys=list(env)):
                post = mock.Mock(side_effect=AssertionError("must not issue requests"))
                code, report, _ = self.run_cli(argv, post, env)
                self.assertEqual(code, 2)
                self.assertEqual(report["status"], "fail")
                post.assert_not_called()


if __name__ == "__main__":
    unittest.main()
