"""
efron's notes:
almost identical to the original uuid7.py from https://github.com/stevesimmons/uuid7.
I simplified the API somewhat, since we only need strings.
The original comments are preserved, and I inlined the MIT license.
"""

"""
Implementation of UUID v7 per the October 2021 draft update
to RFC4122 from 2005:
https://datatracker.ietf.org/doc/html/draft-peabody-dispatch-new-uuid-format

Stephen Simmons, v0.1.0, 2021-12-27

See MIT license reproduced below:
"""

"""
MIT License

Copyright (c) 2021, Stephen Simmons

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
"""
__all__ = (
    "uuid7",
    "uuid7str",
    "time_ns",
    "check_timing_precision",
    "uuid_to_datetime",
)

from typing import Optional
import os
import time


_last = [0, 0, 0, 0]
_last_as_of = [0, 0, 0, 0]


def uuid7(ns: Optional[int] = None) -> str:
    """generate a UUIDv7 as a hexidecimal string.
    uuid7s are based on the current time in nanoseconds since the Unix epoch, adding some bits of randomness and a sequence number to ensure uniqueness.
    See the original code or the RFC for more details.
    """
    global _last, _last_as_of
    if ns is None:
        ns = time.time_ns()
        last = _last
    else:
        last = _last_as_of
        ns = int(ns)  # Fail fast if not an int

    if ns == 0:
        # Special cose for all-zero uuid. Strictly speaking not a UUIDv7.
        t1 = t2 = t3 = t4 = 0
        rand = b"\0" * 6
    else:
        # Treat the first 8 bytes of the uuid as a long (t1) and two ints
        # (t2 and t3) holding 36 bits of whole seconds and 24 bits of
        # fractional seconds.
        # This gives a nominal 60ns resolution, comparable to the
        # timestamp precision in Linux (~200ns) and Windows (100ns ticks).
        sixteen_secs = 16_000_000_000
        t1, rest1 = divmod(ns, sixteen_secs)
        t2, rest2 = divmod(rest1 << 16, sixteen_secs)
        t3, _ = divmod(rest2 << 12, sixteen_secs)
        t3 |= 7 << 12  # Put uuid version in top 4 bits, which are 0 in t3

        # The next two bytes are an int (t4) with two bits for
        # the variant 2 and a 14 bit sequence counter which increments
        # if the time is unchanged.
        if t1 == last[0] and t2 == last[1] and t3 == last[2]:
            # Stop the seq counter wrapping past 0x3FFF.
            # This won't happen in practice, but if it does,
            # uuids after the 16383rd with that same timestamp
            # will not longer be correctly ordered but
            # are still unique due to the 6 random bytes.
            if last[3] < 0x3FFF:
                last[3] += 1
        else:
            last[:] = (t1, t2, t3, 0)
        t4 = (2 << 14) | last[3]  # Put variant 0b10 in top two bits

        # Six random bytes for the lower part of the uuid
        rand = os.urandom(6)

    return f"{t1:>08x}-{t2:>04x}-{t3:>04x}-{t4:>04x}-{rand.hex()}"
