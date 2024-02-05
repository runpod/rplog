"""simple logging package for use with rp-log"""

import enum
import inspect
import json
import os
import sys
import uuid
import trace
from datetime import datetime, timezone
from typing import Any, Optional, TextIO

import friendlyjson


class LogLevel(enum.IntEnum):
    """ Log levels, equivalent to [log::Level](https://docs.rs/log/latest/log/enum.Level.html)
    from rust's [log crate](https://docs.rs/log)"""
    ERROR = 0
    WARN = 1
    INFO = 2
    DEBUG = 3
    TRACE = 4

    @classmethod
    def from_str(cls, s: str) -> "LogLevel":

        string_reprs = {
            cls.ERROR: ("ERROR", "E", "0"),
            cls.WARN: ("WARN", "W", "1"),
            cls.INFO: ("INFO", "I", "2"),
            cls.DEBUG: ("DEBUG", "D", "3"),
            cls.TRACE: ("TRACE", "T", "4"),
        }
        s = s.upper()
        for level, aliases in string_reprs.items():
            if s in aliases:
                return level
        return cls.INFO


_LOG_OUTPUT: TextIO = sys.stderr
_EXTRA_DEBUG_INFO: bool = False
_BASE_ARGS: dict = {}

_ERROR_ENABLED = True
_WARN_ENABLED = True
_INFO_ENABLED = True
_DEBUG_ENABLED = True
_TRACE_ENABLED = False


def init(*, log_level: Optional[LogLevel] = None, log_output: Optional[TextIO] = None, get_callsite: bool = True, metadata: Optional[dict[str, str]] = None, **kwargs):
    """initialize the logger. This should be called before any other functions in this module.
    log_level: the log level to use. If None, defaults to INFO.
    log_output: the file-like object to write logs to. If None, defaults to sys.stderr.
    metadata: additional metadata to include in every log message. if None, the metadata will be loaded from the environment via metadata.load_metadata_from_env.
    get_callsite: if True, include the file, line, and function name of the function that called the log function in the log message.
    kwargs: additional arguments to include in every log message. For example, you might want to include the name of the current process.
    """
    global _LOG_OUTPUT, _BASE_ARGS, _EXTRA_DEBUG_INFO
    if get_callsite:
        _EXTRA_DEBUG_INFO = True
    _BASE_ARGS = {
        "pid": os.getpid(),
        "host": os.uname().nodename,
        # generate a new run id for run so we can cleanly disambiugate logs from different runs
        "run_id": str(uuid.uuid4()),
        **kwargs
    }
    global _DEBUG_ENABLED, _INFO_ENABLED, _WARN_ENABLED, _ERROR_ENABLED, _TRACE_ENABLED
    if log_level is None:
        log_level = LogLevel.INFO
    if log_level == LogLevel.ERROR:
        _TRACE_ENABLED, _DEBUG_ENABLED, _INFO_ENABLED, _WARN_ENABLED, _ERROR_ENABLED = False, False, False, False, True
        return
    if log_level == LogLevel.WARN:
        _TRACE_ENABLED, _DEBUG_ENABLED, _INFO_ENABLED, _WARN_ENABLED, _ERROR_ENABLED = False, False, False, True, True
        return
    if log_level == LogLevel.INFO:
        _TRACE_ENABLED, _DEBUG_ENABLED, _INFO_ENABLED, _WARN_ENABLED, _ERROR_ENABLED = False, False, True, True, True
        return
    if log_level == LogLevel.DEBUG:
        _TRACE_ENABLED, _DEBUG_ENABLED, _INFO_ENABLED, _WARN_ENABLED, _ERROR_ENABLED = False, True, True, True, True
        return
    if log_level == LogLevel.TRACE:
        _TRACE_ENABLED, _DEBUG_ENABLED, _INFO_ENABLED, _WARN_ENABLED, _ERROR_ENABLED = True, True, True, True, True
        return

    if log_output is not None:
        _LOG_OUTPUT = log_output


def log_at(level: LogLevel, msg: str, skip=2, *,  extra_debug_info=False, fp: Optional[TextIO] = None, **kwargs):
    global _LOG_LEVEL
    """
    log a message at the given level.
    If the given level is less than the current log level, this function does nothing.
    - `msg`: the message to log.
    - `kwargs`: additional arguments to include in the log message. they'll be added as JSON fields.
    - `force_flush`: if True, flush the log file after writing the message.
    - `fp`: the file-like object to write logs to. If None, defaults to sys.stderr.
    """

    if extra_debug_info:
        # not log_at, and not debug/info/warn/error/trace, but the function that called that.
        frame = sys._getframe(skip)
        file, line, func, _, _ = inspect.getframeinfo(frame)
        split = file.split("rust-sls-draft/src/py/", 1)
        if len(split) == 2:
            file = split[1]
            if "file_line" not in kwargs:
                kwargs["file_line"] = f"{file}:{line}"
            if "func" not in kwargs:
                kwargs["func"] = func

    if fp is None:
        fp = LOG_OUTPUT  # type: ignore

    # add the trace to the log message
    t = trace.Trace.current()

    obj = {**_BASE_ARGS, **kwargs,
           "msg": msg,
           "level": level.name,
           "request_id": t.request_id,
           "request_source": t.request_source,
           "request_start": t.request_start,
           "time": datetime.now().astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-4]+"Z",
           "trace_id": t.trace_id,
           "trace_source": t.trace_source,
           "trace_start": t.trace_start,
           }

    json.dump(obj, fp, cls=friendlyjson.Encoder)
    fp.write("\n")  # json.dump doesn't add a newline, so we do it manually
    fp.flush()


def debug(msg: str, skip=2, **kwargs):
    if not _DEBUG_ENABLED:
        return
    """log a message at the debug level (3).
    if _LOG_LEVEL is less than 3, this function does nothing.
    kwargs will be added as JSON fields."""
    log_at(LogLevel.DEBUG, msg, skip=skip,
           extra_debug_info=_EXTRA_DEBUG_INFO, **kwargs)


def info(msg: str, skip=2, **kwargs):
    if not _INFO_ENABLED:
        return
    """log a message at the default (info) level (2).
    if _LOG_LEVEL is less than 2, this function does nothing.
    kwargs will be added as JSON fields.
    """
    log_at(LogLevel.INFO, msg, skip=skip,
           extra_debug_info=_EXTRA_DEBUG_INFO, **kwargs)


def warn(msg: str, skip=2, **kwargs):
    if not _WARN_ENABLED:
        return
    """log a message at the warn level (1).
    if _LOG_LEVEL is less than 1, this function does nothing.
    kwargs will be added as JSON fields."""
    log_at(LogLevel.WARN, msg, skip=skip,
           extra_debug_info=_EXTRA_DEBUG_INFO, **kwargs)


def error(msg: str, skip=2, **kwargs):
    global _ERROR_ENABLED
    if not _ERROR_ENABLED:
        return
    """log a message at the error level (0). this is always enabled.
    unlike the other functions in this module, this function flushes the log file after writing the message.
    kwargs will be added as JSON fields."""
    log_at(LogLevel.ERROR, msg, skip=skip,
           extra_debug_info=_EXTRA_DEBUG_INFO, **kwargs)
