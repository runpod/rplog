from dataclasses import dataclass
from datetime import datetime, timezone
from uuid7 import uuid7
from typing import Optional
def as_rfc3339(dt: datetime) -> str:
    return dt.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-4]+"Z"



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
        """
        global _trace
        now = as_rfc3339(datetime.now())

        t = Trace(
            request_id=headers.get("X-Request-ID", str(uuid7())),
            request_source=headers.get("X-Request-Source", "unknown"),
            request_start=headers.get("X-Request-Start", now),
            trace_id=headers.get("X-Trace-ID", str(uuid7())),
            trace_source=headers.get("X-Trace-Source", "unknown"),
            trace_start=headers.get("X-Trace-Start", now)
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
        now = as_rfc3339(datetime.datetime.now())
        global _trace
        t = Trace(
            request_id=str(uuid7.uuid7()),
            request_source="unknown",
            request_start=now,
            trace_id=str(uuid7.uuid7()),
            trace_source="unknown",
            trace_start=now
        )
        _trace = t
        return t
    
    def save_to_headers(self, headers: dict[str, str]) -> None:
        """save the trace to a dictionary of headers in preparation for an HTTP request.
        This creates a new request_id and sets the request_start time to now so that the next service in the chain can add its own trace information."""
        headers["X-Request-ID"] = uuid7()
        headers["X-Request-Source"] = self.request_source
        headers["X-Request-Start"] = as_rfc3339(datetime.now())
        headers["X-Trace-ID"] = self.trace_id

"""the current trace, if any. this is only valid in a truly single-threaded environment."""
_trace: Optional[Trace] = None