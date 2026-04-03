import { describe, it, expect } from "vitest";
import { linkifyText } from "./linkifyText";

describe("linkifyText", () => {
  it("returns plain text when no URLs", () => {
    expect(linkifyText("hello world")).toBe("hello world");
  });

  it("linkifies https URLs", () => {
    const result = linkifyText("check https://example.com please");
    expect(Array.isArray(result)).toBe(true);
    const parts = result as React.ReactNode[];
    expect(parts).toHaveLength(3);
    expect(parts[0]).toBe("check ");
    expect(parts[2]).toBe(" please");
  });

  it("linkifies http URLs", () => {
    const result = linkifyText("go to http://example.com now");
    const parts = result as React.ReactNode[];
    expect(parts).toHaveLength(3);
    expect(parts[0]).toBe("go to ");
  });

  it("linkifies www URLs with https prefix", () => {
    const result = linkifyText("visit www.example.com");
    const parts = result as React.ReactNode[];
    expect(parts).toHaveLength(2);
    expect(parts[0]).toBe("visit ");
  });

  it("strips trailing punctuation from URLs", () => {
    const result = linkifyText("see https://example.com.");
    const parts = result as React.ReactNode[];
    expect(parts).toHaveLength(3);
    expect(parts[0]).toBe("see ");
    // The trailing period should not be in the link
    expect(parts[2]).toBe(".");
  });

  it("handles multiple URLs", () => {
    const result = linkifyText(
      "check https://a.com and https://b.com ok"
    );
    const parts = result as React.ReactNode[];
    expect(parts).toHaveLength(5);
    expect(parts[0]).toBe("check ");
    expect(parts[2]).toBe(" and ");
    expect(parts[4]).toBe(" ok");
  });

  it("handles URL at start of text", () => {
    const result = linkifyText("https://example.com is cool");
    const parts = result as React.ReactNode[];
    expect(parts).toHaveLength(2);
    expect(parts[1]).toBe(" is cool");
  });

  it("handles URL at end of text", () => {
    const result = linkifyText("go to https://example.com");
    const parts = result as React.ReactNode[];
    expect(parts).toHaveLength(2);
    expect(parts[0]).toBe("go to ");
  });

  it("strips trailing comma", () => {
    const result = linkifyText("see https://example.com, thanks");
    const parts = result as React.ReactNode[];
    expect(parts).toHaveLength(3);
    expect(parts[0]).toBe("see ");
    expect(parts[2]).toBe(", thanks");
  });

  it("strips trailing closing paren", () => {
    const result = linkifyText("(https://example.com)");
    const parts = result as React.ReactNode[];
    expect(parts).toHaveLength(3);
    expect(parts[0]).toBe("(");
    expect(parts[2]).toBe(")");
  });
});
