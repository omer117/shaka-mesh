import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Shaka Surf Club — Gear, Conditions & Community",
  description: "Surf gear selected for the coast, local conditions and a community of surfers.",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
