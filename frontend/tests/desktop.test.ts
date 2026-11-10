import assert from "node:assert/strict";
import { test } from "node:test";
import { isPackagedDesktop } from "../src/desktop.ts";

test("packaged pages are desktop before Wails injects environment data", () => {
  for (const address of ["wails://localhost/", "http://wails.localhost/"]) {
    assert.equal(isPackagedDesktop(new URL(address)), true, address);
  }
});

test("browser previews stay previews", () => {
  for (const address of [
    "http://localhost:5173/",
    "http://127.0.0.1:5173/",
    "https://example.com/",
    "file:///tmp/index.html",
  ]) {
    assert.equal(isPackagedDesktop(new URL(address)), false, address);
  }
});
