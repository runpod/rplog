import unittest

from trace import Trace, _resolve_id, _resolve_source, _valid_id


class TestValidId(unittest.TestCase):
    def test_cases(self):
        cases = [
            {"description": "canonical uuid", "input": "5f9c2e6a-1b3d-4c8e-9a0f-2b7c6d5e4f31", "expected": True},
            {"description": "service-prefixed id", "input": "trace_018f3a1c2b4d", "expected": True},
            {"description": "dotted id", "input": "svc.abc.123", "expected": True},
            {"description": "max length accepted", "input": "a" * 200, "expected": True},
            {"description": "empty rejected", "input": "", "expected": False},
            {"description": "one over max rejected", "input": "a" * 201, "expected": False},
            {"description": "CRLF rejected", "input": "abc\r\nX-Evil: 1", "expected": False},
            {"description": "newline rejected", "input": "abc\ndef", "expected": False},
            {"description": "space rejected", "input": "abc def", "expected": False},
            {"description": "control byte rejected", "input": "abc\x00def", "expected": False},
            {"description": "non-ascii rejected", "input": "abcé", "expected": False},
            {"description": "slash rejected", "input": "a/b", "expected": False},
        ]
        for c in cases:
            with self.subTest(c["description"]):
                self.assertEqual(_valid_id(c["input"]), c["expected"])


class TestResolveId(unittest.TestCase):
    def test_cases(self):
        cases = [
            {"description": "valid id propagated", "input": "5f9c2e6a-1b3d-4c8e-9a0f-2b7c6d5e4f31", "propagate": True},
            {"description": "absent id generated", "input": "", "propagate": False},
            {"description": "poisoned id regenerated", "input": "abc\r\nX-Evil: 1", "propagate": False},
            {"description": "over-long id regenerated", "input": "a" * 201, "propagate": False},
        ]
        for c in cases:
            with self.subTest(c["description"]):
                got = _resolve_id(c["input"])
                if c["propagate"]:
                    self.assertEqual(got, c["input"])
                else:
                    self.assertNotEqual(got, c["input"])
                    self.assertTrue(_valid_id(got), f"generated id {got!r} is not valid")


class TestResolveSource(unittest.TestCase):
    def test_cases(self):
        cases = [
            {"description": "empty source becomes unknown", "input": "", "expected": "unknown"},
            {"description": "valid source kept", "input": "runpod-graphql", "expected": "runpod-graphql"},
            {"description": "poisoned source becomes unknown", "input": "svc\r\nX-Evil: 1", "expected": "unknown"},
            {"description": "spaced source becomes unknown", "input": "not a slug", "expected": "unknown"},
        ]
        for c in cases:
            with self.subTest(c["description"]):
                self.assertEqual(_resolve_source(c["input"]), c["expected"])


class TestFromHeaders(unittest.TestCase):
    def test_cases(self):
        good_trace = "5f9c2e6a-1b3d-4c8e-9a0f-2b7c6d5e4f31"
        good_req = "018f3a1c-2b4d-7e8f-9a0b-1c2d3e4f5061"
        cases = [
            {
                "description": "valid ids propagated",
                "headers": {"X-Trace-ID": good_trace, "X-Request-ID": good_req, "X-Trace-Source": "main-ui"},
                "want_trace": good_trace,
                "want_request": good_req,
                "want_trace_source": "main-ui",
            },
            {
                "description": "absent ids generated",
                "headers": {},
                "want_trace": None,
                "want_request": None,
                "want_trace_source": "unknown",
            },
            {
                "description": "poisoned trace id regenerated, valid request id kept",
                "headers": {"X-Trace-ID": "abc\r\nX-Evil: 1", "X-Request-ID": good_req},
                "want_trace": None,
                "want_request": good_req,
                "want_trace_source": "unknown",
            },
            {
                "description": "poisoned source dropped to unknown",
                "headers": {"X-Trace-ID": good_trace, "X-Request-ID": good_req, "X-Trace-Source": "svc\r\nX-Evil"},
                "want_trace": good_trace,
                "want_request": good_req,
                "want_trace_source": "unknown",
            },
        ]
        for c in cases:
            with self.subTest(c["description"]):
                t = Trace.from_headers(dict(c["headers"]))
                if c["want_trace"] is None:
                    self.assertTrue(_valid_id(t.trace_id))
                    self.assertNotIn("\r", t.trace_id)
                else:
                    self.assertEqual(t.trace_id, c["want_trace"])
                if c["want_request"] is None:
                    self.assertTrue(_valid_id(t.request_id))
                else:
                    self.assertEqual(t.request_id, c["want_request"])
                self.assertEqual(t.trace_source, c["want_trace_source"])


class TestSaveToHeaders(unittest.TestCase):
    def test_writes_go_five_headers_and_fresh_request_id(self):
        t = Trace.from_headers({"X-Trace-ID": "5f9c2e6a-1b3d-4c8e-9a0f-2b7c6d5e4f31"})
        original_request_id = t.request_id
        headers: dict[str, str] = {}
        t.save_to_headers(headers)

        for key in ("X-Trace-ID", "X-Request-ID", "X-Trace-Start", "X-Trace-Source", "X-Request-Source"):
            self.assertIn(key, headers)

        self.assertEqual(headers["X-Trace-ID"], t.trace_id)
        # Regression: X-Request-ID must be a plain string, not a tuple (trailing-comma bug).
        self.assertIsInstance(headers["X-Request-ID"], str)
        # A fresh sub-request id, distinct from the inbound one.
        self.assertNotEqual(headers["X-Request-ID"], original_request_id)

    def test_emits_no_unsafe_bytes_from_poisoned_trace(self):
        t = Trace.from_headers({"X-Trace-ID": "abc\r\nX-Evil: 1", "X-Trace-Source": "svc\r\nX-Evil: 2"})
        headers: dict[str, str] = {}
        t.save_to_headers(headers)
        for key, value in headers.items():
            self.assertNotIn("\r", value, f"{key} carries CR")
            self.assertNotIn("\n", value, f"{key} carries LF")


class TestNewRegression(unittest.TestCase):
    def test_new_does_not_raise(self):
        # Regression: new() used datetime.datetime.now() and raised AttributeError.
        t = Trace.new()
        self.assertTrue(_valid_id(t.trace_id))
        self.assertTrue(_valid_id(t.request_id))

    def test_new_generates_unique_ids(self):
        seen = set()
        for _ in range(1000):
            seen.add(Trace.new().trace_id)
        self.assertEqual(len(seen), 1000)


if __name__ == "__main__":
    unittest.main()
