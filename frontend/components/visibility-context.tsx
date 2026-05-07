"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { setPositionsVisibility } from "@/lib/api";

type VisibilityContextType = {
  visible: boolean;
  toggle: () => void;
};

const VisibilityContext = createContext<VisibilityContextType>({
  visible: false,
  toggle: () => {},
});

const STORAGE_KEY = "portfolio-values-visible";

export function VisibilityProvider({ children }: { children: ReactNode }) {
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored !== null) setVisible(stored === "true");
  }, []);

  function toggle() {
    const next = !visible;
    setVisible(next);
    localStorage.setItem(STORAGE_KEY, String(next));
    setPositionsVisibility(next);
  }

  return (
    <VisibilityContext.Provider value={{ visible, toggle }}>
      {children}
    </VisibilityContext.Provider>
  );
}

export function useVisibility() {
  return useContext(VisibilityContext);
}
