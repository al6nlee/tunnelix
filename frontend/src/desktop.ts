// Wails serves packaged macOS/Linux pages over wails://localhost and Windows
// pages over http://wails.localhost. Unlike runtime environment data, this
// address is available before any JavaScript executes.
export function isPackagedDesktop(location: {
  protocol: string;
  hostname: string;
}): boolean {
  return (
    (location.protocol === "wails:" && location.hostname === "localhost") ||
    (location.protocol === "http:" && location.hostname === "wails.localhost")
  );
}
