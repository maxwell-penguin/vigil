import { useEffect, useState } from "react";

// No in-app theme toggle here, so OS preference is the whole story.
export function useIsDark() {
  const query = "(prefers-color-scheme: dark)";
  const [isDark, setIsDark] = useState(
    () => typeof window !== "undefined" && window.matchMedia(query).matches
  );

  useEffect(() => {
    const mql = window.matchMedia(query);
    const onChange = (e) => setIsDark(e.matches);
    mql.addEventListener("change", onChange);
    return () => mql.removeEventListener("change", onChange);
  }, []);

  return isDark;
}
