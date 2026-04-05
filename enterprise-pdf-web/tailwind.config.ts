import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./src/app/**/*.{ts,tsx}",
    "./src/components/**/*.{ts,tsx}",
    "./src/providers/**/*.{ts,tsx}",
    "./src/lib/**/*.{ts,tsx}"
  ],
  theme: {
    extend: {
      colors: {
        ink: "#0f1720",
        shell: "#f4efe7",
        panel: "#fff9f1",
        accent: "#db6b2d",
        mint: "#1d7d66",
        gold: "#d1a94a",
        line: "#d7c7ae"
      },
      boxShadow: {
        panel: "0 14px 40px rgba(15, 23, 32, 0.08)"
      },
      borderRadius: {
        xl2: "1.5rem"
      }
    }
  },
  plugins: []
};

export default config;
