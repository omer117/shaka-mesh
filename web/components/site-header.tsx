"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { LogOut, Menu, ShoppingBag, User, Waves, X } from "lucide-react";
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

const SCROLL_THRESHOLD = 12;

function subscribeScroll(callback: () => void) {
  window.addEventListener("scroll", callback, { passive: true });
  return () => window.removeEventListener("scroll", callback);
}

function useScrolled() {
  return useSyncExternalStore(
    subscribeScroll,
    () => window.scrollY > SCROLL_THRESHOLD,
    () => false,
  );
}

const focusableSelector = 'a[href], button:not([disabled])';

export default function SiteHeader({
  current,
  session,
  onSignIn,
  onSignOut,
  cartCount = 0,
  onCartClick,
}: SiteHeaderProps) {
  const scrolled = useScrolled();
  const [menuOpen, setMenuOpen] = useState(false);
  const [panelOpen, setPanelOpen] = useState(false);
  const avatarRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const hamburgerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);

  const closeMenu = useCallback((restoreFocus: boolean) => {
    setMenuOpen(false);
    if (restoreFocus) avatarRef.current?.focus();
  }, []);

  const closePanel = useCallback((restoreFocus: boolean) => {
    setPanelOpen(false);
    if (restoreFocus) hamburgerRef.current?.focus();
  }, []);

  // Account dropdown: outside click, Escape, focus first item on open.
  useEffect(() => {
    if (!menuOpen) return;
    menuRef.current?.querySelector<HTMLElement>('[role="menuitem"]')?.focus();
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node;
      if (!menuRef.current?.contains(target) && !avatarRef.current?.contains(target)) {
        setMenuOpen(false);
      }
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") closeMenu(true);
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [menuOpen, closeMenu]);

  // Mobile panel: Escape, focus trap, scroll lock.
  useEffect(() => {
    if (!panelOpen) return;
    const panel = panelRef.current;
    panel?.querySelector<HTMLElement>(focusableSelector)?.focus();
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        closePanel(true);
        return;
      }
      if (event.key !== "Tab" || !panel) return;
      const items = panel.querySelectorAll<HTMLElement>(focusableSelector);
      if (items.length === 0) return;
      const first = items[0];
      const last = items[items.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [panelOpen, closePanel]);

  const onMenuKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    const items = Array.from(
      event.currentTarget.querySelectorAll<HTMLElement>('[role="menuitem"]'),
    );
    const index = items.indexOf(document.activeElement as HTMLElement);
    if (event.key === "ArrowDown") {
      event.preventDefault();
      items[(index + 1) % items.length]?.focus();
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      items[(index - 1 + items.length) % items.length]?.focus();
    } else if (event.key === "Home") {
      event.preventDefault();
      items[0]?.focus();
    } else if (event.key === "End") {
      event.preventDefault();
      items[items.length - 1]?.focus();
    } else if (event.key === "Tab") {
      setMenuOpen(false);
    }
  };

  const initial = session?.email[0]?.toUpperCase() ?? "S";

  return (
    <header className={`site-nav ${scrolled || panelOpen ? "is-solid" : ""}`}>
      <div className="site-nav-inner">
        <Link className="site-nav-brand" href="/" aria-label="Shaka home">
          <span className="site-nav-mark"><Waves size={17} aria-hidden="true" /></span>
          <span>shaka<span>.</span></span>
        </Link>

        <nav className="site-nav-links" aria-label="Main navigation">
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

        <div className="site-nav-actions">
          {onCartClick && (
            <button
              type="button"
              className="site-nav-icon-button"
              onClick={onCartClick}
              aria-label={`Open bag, ${cartCount} ${cartCount === 1 ? "item" : "items"}`}
            >
              <ShoppingBag size={19} aria-hidden="true" />
              {cartCount > 0 && <b aria-hidden="true">{cartCount > 99 ? "99+" : cartCount}</b>}
            </button>
          )}

          <div className="site-nav-account">
            {session ? (
              <>
                <button
                  ref={avatarRef}
                  type="button"
                  className="site-nav-avatar"
                  aria-label="Account menu"
                  aria-haspopup="menu"
                  aria-expanded={menuOpen}
                  aria-controls="site-nav-account-menu"
                  onClick={() => setMenuOpen((open) => !open)}
                  onKeyDown={(event) => {
                    if (event.key === "ArrowDown" && !menuOpen) {
                      event.preventDefault();
                      setMenuOpen(true);
                    }
                  }}
                >
                  {initial}
                </button>
                {menuOpen && (
                  <div
                    ref={menuRef}
                    id="site-nav-account-menu"
                    className="site-nav-menu"
                    role="menu"
                    aria-label="Account"
                    onKeyDown={onMenuKeyDown}
                  >
                    <p className="site-nav-menu-email">{session.email}</p>
                    <Link
                      role="menuitem"
                      href="/profile"
                      onClick={() => setMenuOpen(false)}
                    >
                      <User size={15} aria-hidden="true" /> Profile
                    </Link>
                    <button
                      type="button"
                      role="menuitem"
                      onClick={() => {
                        closeMenu(false);
                        onSignOut();
                      }}
                    >
                      <LogOut size={15} aria-hidden="true" /> Sign out
                    </button>
                  </div>
                )}
              </>
            ) : (
              <button type="button" className="site-nav-signin" onClick={onSignIn}>
                Sign in
              </button>
            )}
          </div>

          <button
            ref={hamburgerRef}
            type="button"
            className="site-nav-icon-button site-nav-hamburger"
            aria-label="Open menu"
            aria-expanded={panelOpen}
            aria-controls="site-nav-panel"
            onClick={() => setPanelOpen(true)}
          >
            <Menu size={20} aria-hidden="true" />
          </button>
        </div>
      </div>

      <div
        className={`site-nav-backdrop ${panelOpen ? "open" : ""}`}
        onClick={() => closePanel(true)}
        aria-hidden="true"
      />
      <div
        ref={panelRef}
        id="site-nav-panel"
        className={`site-nav-panel ${panelOpen ? "open" : ""}`}
        role="dialog"
        aria-modal="true"
        aria-label="Menu"
        inert={!panelOpen}
      >
        <button
          type="button"
          className="site-nav-icon-button site-nav-panel-close"
          aria-label="Close menu"
          onClick={() => closePanel(true)}
        >
          <X size={20} aria-hidden="true" />
        </button>
        <nav aria-label="Mobile navigation">
          {links.map((link) => (
            <Link
              key={link.key}
              href={link.href}
              className={current === link.key ? "active" : ""}
              aria-current={current === link.key ? "page" : undefined}
              onClick={() => closePanel(false)}
            >
              {link.label}
            </Link>
          ))}
        </nav>
        <div className="site-nav-panel-account">
          {session ? (
            <>
              <p>{session.email}</p>
              <Link href="/profile" onClick={() => closePanel(false)}>
                <User size={16} aria-hidden="true" /> Profile
              </Link>
              <button
                type="button"
                onClick={() => {
                  closePanel(false);
                  onSignOut();
                }}
              >
                <LogOut size={16} aria-hidden="true" /> Sign out
              </button>
            </>
          ) : (
            <button
              type="button"
              className="site-nav-signin"
              onClick={() => {
                closePanel(false);
                onSignIn();
              }}
            >
              Sign in
            </button>
          )}
        </div>
      </div>
    </header>
  );
}
