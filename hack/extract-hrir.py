#!/usr/bin/env python3
"""Pull the directions occam needs out of a SADIE II SOFA file.

    uv run --with h5py python3 hack/extract-hrir.py D1_48K_24bit_256tap_FIR_SOFA.sofa

Writes internal/spatial/hrir/sadie48.bin. Every direction should report 0.00
degrees of error; anything else means the layouts gained a speaker angle the
measurement grid does not contain.
"""
import math
import struct
import sys

import h5py
import numpy as np

# Clockwise-positive azimuth, matching spatial.Speaker. SADIE is
# counter-clockwise, so it is negated on the way in.
WANTED = [
    (0, 0), (-30, 0), (30, 0), (-90, 0), (90, 0), (-110, 0), (110, 0),
    (-150, 0), (150, 0), (-45, 45), (45, 45), (-135, 45), (135, 45),
]
OUT = "internal/spatial/hrir/sadie48.bin"


def unit(az, el):
    a, e = math.radians(az), math.radians(el)
    return np.array([math.cos(e) * math.cos(a), math.cos(e) * math.sin(a), math.sin(e)])


def main(path):
    f = h5py.File(path, "r")
    ir = f["Data.IR"][:]
    pos = f["SourcePosition"][:]
    rate = int(f["Data.SamplingRate"][0])
    taps = ir.shape[2]

    grid = np.stack([unit(p[0], p[1]) for p in pos])

    records = []
    for az, el in WANTED:
        dots = grid @ unit((-az) % 360, el)
        i = int(np.argmax(dots))
        err = math.degrees(math.acos(min(1.0, max(-1.0, dots[i]))))
        print("  %+7.1f az %+6.1f el -> %+7.2f %+6.2f  off by %.2f deg"
              % (az, el, pos[i][0], pos[i][1], err))
        records.append((az, el, ir[i, 0].astype(np.float32), ir[i, 1].astype(np.float32)))

    with open(OUT, "wb") as out:
        out.write(b"OCHR")
        out.write(struct.pack("<IIII", 1, rate, taps, len(records)))
        for az, el, left, right in records:
            out.write(struct.pack("<ff", az, el))
            out.write(left.tobytes())
            out.write(right.tobytes())
    print("wrote %s: %d directions, %d taps @ %d Hz" % (OUT, len(records), taps, rate))


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    main(sys.argv[1])
