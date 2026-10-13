"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import {
  ArrowRight,
  Check,
  LoaderCircle,
  Minus,
  Plus,
  ShoppingBag,
  Sparkles,
  Waves,
  X,
} from "lucide-react";
import AuthDialog from "@/components/auth-dialog";
import SiteHeader from "@/components/site-header";
import {
  addCartItem,
  ApiError,
  checkout,
  getCart,
  getProducts,
  removeCartItem,
  type Cart,
  type Product,
  type ShopOrder,
} from "@/lib/api";
import { useAuthSession } from "@/lib/use-auth-session";

const emptyCart: Cart = { items: [], total_cents: 0 };

function money(cents: number) {
  return new Intl.NumberFormat("en-IL", {
    style: "currency",
    currency: "ILS",
    maximumFractionDigits: 0,
  }).format(cents / 100);
}

function productIcon(category: string) {
  if (category === "Surfboards") return "board";
  if (category === "Wetsuits") return "wetsuit";
  if (category === "Fins") return "fins";
  if (category === "Apparel") return "apparel";
  return "accessories";
}

function ProductArtwork({ product }: { product: Product }) {
  return (
    <div className={`product-art art-${productIcon(product.category)}`}>
      {product.image_url ? (
        // Product imagery is merchant-managed content and may be hosted externally.
        // eslint-disable-next-line @next/next/no-img-element
        <img src={product.image_url} alt="" />
      ) : (
        <>
          <span className="art-sun" />
          <span className="art-wave art-wave-one" />
          <span className="art-wave art-wave-two" />
          <span className="art-product-shape">
            {productIcon(product.category) === "board" && <Waves size={42} />}
            {productIcon(product.category) === "wetsuit" && <span className="wetsuit-silhouette" />}
            {productIcon(product.category) === "fins" && <span className="fin-silhouette" />}
            {productIcon(product.category) === "apparel" && <span className="poncho-silhouette" />}
            {productIcon(product.category) === "accessories" && <Sparkles size={34} />}
          </span>
        </>
      )}
      {product.featured && <span className="product-art-tag">SHAPER’S PICK</span>}
    </div>
  );
}

export default function ShopPage() {
  const { session, ready, notice, signIn, signOut, setNotice } = useAuthSession();
  const [products, setProducts] = useState<Product[]>([]);
  const [cart, setCart] = useState<Cart>(emptyCart);
  const [category, setCategory] = useState("All gear");
  const [loading, setLoading] = useState(true);
  const [cartOpen, setCartOpen] = useState(false);
  const [authOpen, setAuthOpen] = useState(false);
  const [shopError, setShopError] = useState("");
  const [order, setOrder] = useState<ShopOrder | null>(null);
  const [busyProduct, setBusyProduct] = useState("");
  const [checkingOut, setCheckingOut] = useState(false);

  useEffect(() => {
    let active = true;
    getProducts()
      .then((result) => {
        if (active) setProducts(result);
      })
      .catch((caught: unknown) => {
        if (active) {
          setShopError(
            caught instanceof ApiError
              ? caught.message
              : "Could not load the shop. Check your connection and try again.",
          );
        }
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  const refreshCart = useCallback(async (accessToken: string) => {
    const updated = await getCart(accessToken);
    setCart(updated);
    return updated;
  }, []);

  useEffect(() => {
    let active = true;
    if (!ready || !session) return;
    getCart(session.accessToken)
      .then((updated) => {
        if (active) setCart(updated);
      })
      .catch((caught: unknown) => {
        if (active) {
          setNotice(
            caught instanceof ApiError
              ? caught.message
              : "Could not load your saved cart.",
          );
        }
      });
    return () => {
      active = false;
    };
  }, [ready, session, setNotice]);

  const categories = useMemo(
    () => ["All gear", ...new Set(products.map((product) => product.category))],
    [products],
  );
  const visibleProducts =
    category === "All gear"
      ? products
      : products.filter((product) => product.category === category);
  const displayedCart = session ? cart : emptyCart;
  const itemCount = displayedCart.items.reduce((sum, item) => sum + item.quantity, 0);

  const addToBag = async (product: Product) => {
    if (!session) {
      setAuthOpen(true);
      return;
    }
    setBusyProduct(product.id);
    setShopError("");
    try {
      await addCartItem(product.id, 1, session.accessToken).then(setCart);
    } catch (caught) {
      setShopError(
        caught instanceof ApiError ? caught.message : "Could not add that item to your bag.",
      );
    } finally {
      setBusyProduct("");
    }
  };

  const removeFromBag = async (productId: string) => {
    if (!session) return;
    try {
      await removeCartItem(productId, session.accessToken);
      await refreshCart(session.accessToken);
    } catch (caught) {
      setNotice(
        caught instanceof ApiError ? caught.message : "Could not update your bag.",
      );
    }
  };

  const completeCheckout = async () => {
    if (!session) {
      setAuthOpen(true);
      return;
    }
    setCheckingOut(true);
    setShopError("");
    try {
      setOrder(await checkout(session.accessToken));
      setCart(emptyCart);
      setCartOpen(false);
    } catch (caught) {
      setShopError(
        caught instanceof ApiError ? caught.message : "Checkout could not be completed.",
      );
    } finally {
      setCheckingOut(false);
    }
  };

  const handleSignOut = () => {
    signOut();
    setCart(emptyCart);
  };

  return (
    <main className="commerce-page">
      <SiteHeader
        current="shop"
        session={session}
        onSignIn={() => setAuthOpen(true)}
        onSignOut={handleSignOut}
        cartCount={itemCount}
        onCartClick={() => setCartOpen(true)}
      />
      {(notice || shopError) && (
        <div className={`commerce-notice ${shopError ? "error" : ""}`} role="status">
          <span>{shopError || notice}</span>
          <button
            aria-label="Dismiss message"
            onClick={() => {
              setShopError("");
              setNotice("");
            }}
          >
            <X size={15} />
          </button>
        </div>
      )}

      <section className="shop-hero">
        <div className="hero-copy">
          <span className="shop-eyebrow"><i /> THE SHAKA SURF SHOP</span>
          <h1>Good gear.<br /><em>Better days.</em></h1>
          <p>
            Hand-picked essentials for early alarms, saltwater rituals and the
            long way home.
          </p>
          <a className="hero-cta" href="#shop">
            Shop the collection <ArrowRight size={16} />
          </a>
          <div className="hero-promise">
            <span><Check size={13} /> Curated by surfers</span>
            <span><Check size={13} /> Built for the coast</span>
          </div>
        </div>
        <div className="hero-art" aria-label="Illustration of a surfboard by the sea">
          <span className="hero-sun" />
          <span className="hero-horizon" />
          <span className="hero-water hero-water-back" />
          <span className="hero-water hero-water-front" />
          <span className="hero-board"><i /><b /><small /></span>
          <span className="hero-stamp">GOOD<br />TIDES<br /><i>✳</i></span>
          <span className="hero-caption">MADE FOR THE IN-BETWEEN</span>
        </div>
        <span className="hero-side-note">EST. BY THE SEA · 2026</span>
      </section>

      <section className="shop-benefits" aria-label="Shop benefits">
        <div><span>01</span><b>Small-batch selection</b><i>Less noise, better gear.</i></div>
        <div><span>02</span><b>Local surf knowledge</b><i>Picked for real conditions.</i></div>
        <div><span>03</span><b>Community first</b><i>Good things travel together.</i></div>
      </section>

      <section className="shop-collection" id="shop">
        <div className="collection-heading">
          <div>
            <span className="shop-eyebrow">THE LINEUP</span>
            <h2>Gear for the good days.</h2>
          </div>
          <p>Thoughtful essentials, selected for life in and around the water.</p>
        </div>
        <div className="shop-toolbar">
          <div className="category-filter" aria-label="Filter products">
            {categories.map((item) => (
              <button
                key={item}
                className={category === item ? "active" : ""}
                onClick={() => setCategory(item)}
              >
                {item}
              </button>
            ))}
          </div>
          <span>{loading ? "Loading collection…" : `${visibleProducts.length} pieces`}</span>
        </div>
        {loading ? (
          <div className="shop-state"><LoaderCircle className="spin" size={20} /> Loading the lineup…</div>
        ) : visibleProducts.length === 0 ? (
          <div className="shop-state">{shopError || "No gear found in this collection."}</div>
        ) : (
          <div className="product-grid">
            {visibleProducts.map((product) => (
              <article className="product-card" key={product.id}>
                <ProductArtwork product={product} />
                <div className="product-meta">
                  <span>{product.category}</span>
                  <span>{product.inventory > 0 ? "In the lineup" : "Sold out"}</span>
                </div>
                <h3>{product.name}</h3>
                <p>{product.description}</p>
                <div className="product-buy-row">
                  <strong>{money(product.price_cents)}</strong>
                  <button
                    onClick={() => void addToBag(product)}
                    disabled={product.inventory === 0 || busyProduct === product.id}
                  >
                    {busyProduct === product.id ? (
                      <LoaderCircle size={15} className="spin" />
                    ) : (
                      <Plus size={15} />
                    )}
                    Add to bag
                  </button>
                </div>
              </article>
            ))}
          </div>
        )}
      </section>

      <section className="shop-community-promo">
        <div>
          <span className="shop-eyebrow">MORE THAN THE GEAR</span>
          <h2>Good sessions start<br />with good people.</h2>
          <p>Trade local knowledge, plan a dawn patrol and find your crew.</p>
        </div>
        <Link href="/forum">Meet the community <ArrowRight size={15} /></Link>
      </section>

      <footer className="commerce-footer">
        <span>shaka<span>.</span> SURF CLUB</span>
        <span>GOOD TIDES, GOOD PEOPLE.</span>
        <Link href="/conditions">Check today’s conditions <ArrowRight size={13} /></Link>
      </footer>

      {cartOpen && (
        <div className="drawer-backdrop" onMouseDown={() => setCartOpen(false)}>
          <aside
            className="cart-drawer"
            aria-label="Shopping bag"
            onMouseDown={(event) => event.stopPropagation()}
          >
            <div className="drawer-heading">
              <div><span className="shop-eyebrow">YOUR LINEUP</span><h2>Shopping bag</h2></div>
              <button aria-label="Close bag" onClick={() => setCartOpen(false)}><X size={19} /></button>
            </div>
            {!session ? (
              <div className="cart-empty">
                <ShoppingBag size={27} />
                <p>Sign in to see and save your bag.</p>
                <button onClick={() => { setCartOpen(false); setAuthOpen(true); }}>Sign in</button>
              </div>
            ) : displayedCart.items.length === 0 ? (
              <div className="cart-empty">
                <ShoppingBag size={27} />
                <p>Your bag is waiting for a good find.</p>
                <button onClick={() => setCartOpen(false)}>Explore the shop</button>
              </div>
            ) : (
              <>
                <div className="cart-lines">
                  {displayedCart.items.map((item) => (
                    <article className="cart-line" key={item.product.id}>
                      <ProductArtwork product={item.product} />
                      <div className="cart-line-details">
                        <span>{item.product.category}</span>
                        <strong>{item.product.name}</strong>
                        <small>Qty {item.quantity} · {money(item.product.price_cents)}</small>
                        <button onClick={() => void removeFromBag(item.product.id)}>
                          <Minus size={12} /> Remove
                        </button>
                      </div>
                      <b>{money(item.line_cents)}</b>
                    </article>
                  ))}
                </div>
                <div className="cart-checkout">
                  <div><span>Subtotal</span><strong>{money(displayedCart.total_cents)}</strong></div>
                  <small>Taxes and delivery are not charged in this demo checkout.</small>
                  <button onClick={() => void completeCheckout()} disabled={checkingOut}>
                    {checkingOut ? <LoaderCircle size={16} className="spin" /> : <Check size={16} />}
                    Complete demo checkout
                  </button>
                </div>
              </>
            )}
          </aside>
        </div>
      )}

      {authOpen && (
        <AuthDialog
          onClose={() => setAuthOpen(false)}
          onAuthenticated={(nextSession) => {
            signIn(nextSession);
            setAuthOpen(false);
          }}
        />
      )}
      {order && (
        <div className="dialog-backdrop" onMouseDown={() => setOrder(null)}>
          <section className="dialog-card order-confirmation" role="dialog" aria-modal="true">
            <button className="dialog-close" aria-label="Close" onClick={() => setOrder(null)}><X size={18} /></button>
            <span className="order-check"><Check size={22} /></span>
            <div className="dialog-eyebrow">DEMO CHECKOUT COMPLETE</div>
            <h2>Good choice.</h2>
            <p className="dialog-copy">
              Order {order.id.slice(0, 8)} is saved. No payment was collected.
            </p>
            <button className="dialog-submit" onClick={() => setOrder(null)}>Back to the shop</button>
          </section>
        </div>
      )}
    </main>
  );
}
