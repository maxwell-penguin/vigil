/** @type {import('tailwindcss').Config} */
export default {
  darkMode: "class", // light is the product default; no OS-driven theme switch
  content: ["./index.html", "./src/**/*.{js,jsx}"],
  theme: {
    extend: {
      fontFamily: {
        sans: ["Inter", "system-ui", "sans-serif"],
        mono: ["JetBrains Mono", "ui-monospace", "monospace"],
      },
      colors: {
        subtle: "#f9fafb",
        line: { DEFAULT: "#e5e7eb", hover: "#d1d5db" },
        ink: { DEFAULT: "#111827", secondary: "#6b7280", muted: "#9ca3af" },
        ok: { DEFAULT: "#16a34a", soft: "#dcfce7" },
        warn: { DEFAULT: "#d97706", soft: "#fef3c7" },
        danger: { DEFAULT: "#dc2626", soft: "#fee2e2" },
        accent: { DEFAULT: "#2563eb", soft: "#eff6ff" },
      },
    },
  },
  plugins: [],
};
