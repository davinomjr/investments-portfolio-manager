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
    label: "International",
    color: "#9333ea",
    soft: "#f3e8ff",
    border: "#d8b4fe",
    text: "#7e22ce",
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
  bdr: {
    label: "BDR",
    color: "#0891b2",
    soft: "#cffafe",
    border: "#67e8f9",
    text: "#0e7490",
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
