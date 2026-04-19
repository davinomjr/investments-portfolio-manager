const ASSET_STYLES: Record<
  string,
  {
    label: string;
    color: string;
    soft: string;
    border: string;
    text: string;
  }
> = {
  stock: {
    label: "Stock",
    color: "#2563eb",
    soft: "#dbeafe",
    border: "#93c5fd",
    text: "#1d4ed8",
  },
  etf: {
    label: "ETF",
    color: "#7c3aed",
    soft: "#ede9fe",
    border: "#c4b5fd",
    text: "#6d28d9",
  },
  international_etf: {
    label: "Intl ETF",
    color: "#9333ea",
    soft: "#f3e8ff",
    border: "#d8b4fe",
    text: "#7e22ce",
  },
  international_stock: {
    label: "Intl Stock",
    color: "#a855f7",
    soft: "#faf5ff",
    border: "#e9d5ff",
    text: "#9333ea",
  },
  international_bond: {
    label: "Intl Bond",
    color: "#7c3aed",
    soft: "#ede9fe",
    border: "#c4b5fd",
    text: "#6d28d9",
  },
  etf_or_fii: {
    label: "ETF / FII",
    color: "#4f46e5",
    soft: "#e0e7ff",
    border: "#a5b4fc",
    text: "#4338ca",
  },
  fund: {
    label: "Fund",
    color: "#ea580c",
    soft: "#ffedd5",
    border: "#fdba74",
    text: "#c2410c",
  },
  government_bond: {
    label: "Gov Bond",
    color: "#16a34a",
    soft: "#dcfce7",
    border: "#86efac",
    text: "#15803d",
  },
  fii: {
    label: "FII",
    color: "#0891b2",
    soft: "#cffafe",
    border: "#67e8f9",
    text: "#0e7490",
  },
  bdr: {
    label: "BDR",
    color: "#0284c7",
    soft: "#e0f2fe",
    border: "#7dd3fc",
    text: "#0369a1",
  },
  other: {
    label: "Other",
    color: "#525252",
    soft: "#f5f5f5",
    border: "#d4d4d4",
    text: "#404040",
  },
};

export function getAssetStyle(assetType: string) {
  return ASSET_STYLES[assetType] ?? ASSET_STYLES.other;
}

const FRIENDLY_LABEL_TYPES = new Set(["government_bond", "other"]);

export function formatHoldingLabel(
  ticker: string,
  companyName: string | null | undefined,
  assetType: string,
): string {
  if (!FRIENDLY_LABEL_TYPES.has(assetType)) return ticker;
  const name = (companyName ?? "").trim();
  if (!name) return ticker;
  return name.replace(/\bcom\s+Juros\s+Semestrais\b/i, "c/ Juros Semestrais").trim();
}
