"use client";

import Link from "next/link";
import { ShoppingBag, Waves } from "lucide-react";
import type { AuthSession } from "@/lib/api";

interface SiteHeaderProps {
  current: "shop" | "conditions" | "forum";
  session: AuthSession | null;
  onSignIn: () => void;
  onSignOut: () => void;
  cartCount?: number;
  onCartClick?: () => void;
}

const links = [
  { key: "shop", href: "/", label: "Shop" },
  { key: "conditions", href: "/conditions", label: "Surf conditions" },
  { key: "forum", href: "/forum", label: "Community" },
] as const;

export default function SiteHeader({
  current,
  session,
  onSignIn,
  onSignOut,
  cartCount = 0,
  onCartClick,
}: SiteHeaderProps) {
  return (
    <header className="commerce-header">
      <Link className="commerce-brand" href="/" aria-label="Shaka home">
        <span className="commerce-brand-mark"><Waves size={18} /></span>
        <span>shaka<span>.</span></span>
      </Link>
      <nav className="commerce-nav" aria-label="Main navigation">
        {links.map((link) => (
          <Link
            key={link.key}
            href={link.href}
            className={current === link.key ? "active" : ""}
            aria-current={current === link.key ? "page" : undefined}
          >
            {link.label}
          </Link>
        ))}
      </nav>
      <div className="commerce-header-actions">
        {onCartClick && (
          <button className="header-cart-button" onClick={onCartClick}>
            <ShoppingBag size={17} />
            <span>Bag</span>
            <b>{cartCount}</b>
          </button>
        )}
        {session ? (
          <button className="header-account-button" onClick={onSignOut}>
            <span>{session.email[0]?.toUpperCase() ?? "S"}</span>
            <span className="header-account-label">Sign out</span>
          </button>
        ) : (
          <button className="header-signin-button" onClick={onSignIn}>
            Sign in
          </button>
        )}
      </div>
    </header>
  );
}
