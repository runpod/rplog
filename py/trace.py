import re
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Optional

from uuid7 import uuid7


def as_rfc3339(dt: datetime) -> str:
    return dt.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-4] + "Z"


# An inbound id may be at most _MAX_ID_LEN chars and contain only [A-Za-z0-9._-].
# Anything else (empty, over-long, or carrying CR/LF, control, or non-ascii bytes)
# is untrusted: an id is regenerated and a source is dropped, so a poisoned inbound
# header can never be re-emitted by save_to_headers to corrupt a downstream request
# or a log line. Mirrors validID in the Go trace package.
_MAX_ID_LEN = 200
_VALID_ID = re.compile(r"\A[A-Za-z0-9._-]+\Z")


def _valid_id(s: str) -> bool:
    return bool(s) and len(s) <= _MAX_ID_LEN and _VALID_ID.match(s) is not None


def _resolve_id(s: str) -> str:
    return s if _valid_id(s) else uuid7()


def _resolve_source(s: str) -> str:
    return s if _valid_id(s) else "unknown"


@dataclass
class Trace:
    """A trace object that can be used to track a request through multiple services.
    See the overall rp-log documentation for more details."""

    request_id: str  # unique to this service and trace_id
    request_source: str
    request_start: str  # when the request started, in RFC3339 format
    trace_id: str  # may span multiple services
    trace_source: str  # the service that started the trace
    trace_start: str  # when the trace started, in RFC3339 format

    @staticmethod
    def from_headers(headers: dict[str, str]) -> "Trace":
        """get a trace from a dictionary of headers, or create a new one if it doesn't exist.
        this will over-write the global trace.

        Inbound header values are untrusted: an absent or malformed X-Trace-ID /
        X-Request-ID is replaced with a fresh uuid, and a malformed source is
        dropped to "unknown" (see _valid_id).
        """
        global _trace
        now = as_rfc3339(datetime.now())

        t = Trace(
            request_id=_resolve_id(headers.get("X-Request-ID", "")),
            request_source=_resolve_source(headers.get("X-Request-Source", "")),
            request_start=headers.get("X-Request-Start") or now,
            trace_id=_resolve_id(headers.get("X-Trace-ID", "")),
            trace_source=_resolve_source(headers.get("X-Trace-Source", "")),
            trace_start=headers.get("X-Trace-Start") or now,
        )
        _trace = t
        return t

    @classmethod
    def current(cls) -> "Trace":
        """get the current trace. if none exists, this will create a new one.
        this function is only safe to use in a single-threaded environment:
        if you are using asyncio or other concurrency features,
        you will need to pass the trace object around explicitly or use
        something like flask's request context to store it."""
        global _trace
        if _trace is not None:
            return _trace
        _trace = Trace.new()
        return _trace

    @staticmethod
    def new():
        """start a fresh trace and return it, overwriting the global trace if it exists."""
        now = as_rfc3339(datetime.now())
        global _trace
        t = Trace(
            request_id=uuid7(),
            request_source="unknown",
            request_start=now,
            trace_id=uuid7(),
            trace_source="unknown",
            trace_start=now,
        )
        _trace = t
        return t

    def save_to_headers(self, headers: dict[str, str]) -> None:
        """save the trace to a dictionary of headers in preparation for an HTTP request.

        Writes the same five headers as the Go trace.SaveToHeader, and mints a fresh
        X-Request-ID: this is a new request within the same trace, so the trace_id
        persists across the hop while the request_id identifies this sub-request.
        """
        headers["X-Trace-ID"] = self.trace_id
        headers["X-Request-ID"] = uuid7()
        headers["X-Trace-Start"] = self.trace_start
        headers["X-Trace-Source"] = self.trace_source
        headers["X-Request-Source"] = self.request_source


"""the current trace, if any. this is only valid in a truly single-threaded environment."""
_trace: Optional[Trace] = None
