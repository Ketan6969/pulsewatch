import { describe, expect, it } from "vitest";
import { bitrate, bytes, duration, interval, ms, pct, relative, utilTone } from "./format";

describe("format", () => {
  it("formats percentages and missing values", () => {
    expect(pct(42.345)).toBe("42.3%");
    expect(pct(null)).toBe("—");
  });
  it("formats bytes with binary units", () => {
    expect(bytes(512)).toBe("512 B");
    expect(bytes(1536)).toBe("1.5 KB");
    expect(bytes(8 * 1024 ** 3)).toBe("8.0 GB");
  });
  it("formats bitrates from bytes/s", () => {
    expect(bitrate(125)).toBe("1.0 Kbps");
    expect(bitrate(12_500_000)).toBe("100 Mbps");
  });
  it("formats durations", () => {
    expect(duration(59)).toBe("59s");
    expect(duration(3661)).toBe("1h 1m");
    expect(duration(90061)).toBe("1d 1h");
    expect(ms(250)).toBe("250 ms");
    expect(ms(12500)).toBe("12.5 s");
    expect(interval(300)).toBe("5m");
  });
  it("formats relative times", () => {
    const now = Date.parse("2026-01-01T00:10:00Z");
    expect(relative("2026-01-01T00:09:30Z", now)).toBe("30s ago");
    expect(relative("2026-01-01T00:00:00Z", now)).toBe("10m ago");
    expect(relative(undefined)).toBe("—");
  });
  it("derives utilization tone", () => {
    expect(utilTone(50)).toBeUndefined();
    expect(utilTone(80)).toBe("warning");
    expect(utilTone(95)).toBe("danger");
  });
});
