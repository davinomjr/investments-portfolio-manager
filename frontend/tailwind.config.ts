import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./app/**/*.{js,ts,jsx,tsx,mdx}",
    "./components/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  theme: {
    extend: {
      colors: {
        ink: "#0f172a",
        sand: "#f5efe5",
        clay: "#e7d5b7",
        pine: "#22c55e",
        gold: "#be8c2f",
      },
      fontFamily: {
        sans: ["Georgia", "ui-serif", "serif"],
      },
    },
  },
  plugins: [],
};

export default config;

