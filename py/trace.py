import re
from dataclasses import dataclass
from datetime import datetime, timezone

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
    # Match Go's resolveSource: keep a valid source, drop anything else (including
    # empty) to "" so save_to_headers can never re-emit unsafe bytes.
    return s if _valid_id(s) else ""


def _resolve_start(s: str, now: datetime) -> str:
    # Parse an inbound RFC3339 timestamp and re-emit it canonically, falling back
    # to `now` when the header is absent, malformed, or in the future. Clamping a
    # future value to now silently (like a malformed one) mirrors Go's
    # FromHeaderOrNew and stops a caller from choosing the trace's start time.
    # Re-formatting through as_rfc3339 also means the stored value can never carry
    # CR/LF (or any other injected bytes) into save_to_headers.
    if s:
        try:
            parsed = datetime.fromisoformat(s.replace("Z", "+00:00"))
            if parsed.tzinfo is None:
                parsed = parsed.replace(tzinfo=timezone.utc)
            if parsed <= now:
                return as_rfc3339(parsed)
        except ValueError:
            pass
    return as_rfc3339(now)


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
        X-Request-ID is replaced with a fresh uuid, a malformed source is dropped
        to "", and a malformed X-Trace-Start falls back to now (see _valid_id /
        _resolve_start). There is no X-Request-Start header — the request timing
        starts when the server receives the request, matching Go's SaveToHeader.
        """
        global _trace
        now_dt = datetime.now(timezone.utc)
        now = as_rfc3339(now_dt)

        t = Trace(
            request_id=_resolve_id(headers.get("X-Request-ID", "")),
            request_source=_resolve_source(headers.get("X-Request-Source", "")),
            request_start=now,
            trace_id=_resolve_id(headers.get("X-Trace-ID", "")),
            trace_source=_resolve_source(headers.get("X-Trace-Source", "")),
            trace_start=_resolve_start(headers.get("X-Trace-Start", ""), now_dt),
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
    def new() -> "Trace":
        """start a fresh trace and return it, overwriting the global trace if it exists."""
        now = as_rfc3339(datetime.now(timezone.utc))
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

        Every stored field is already validated at construction (ids charset-checked,
        source dropped to "" if invalid, trace_start re-formatted through as_rfc3339),
        so no value written here can carry CR/LF into an outbound header.
        """
        headers["X-Trace-ID"] = self.trace_id
        headers["X-Request-ID"] = uuid7()
        headers["X-Trace-Start"] = self.trace_start
        headers["X-Trace-Source"] = self.trace_source
        headers["X-Request-Source"] = self.request_source


"""the current trace, if any. this is only valid in a truly single-threaded environment."""
_trace: "Trace | None" = None
