CREATE TABLE IF NOT EXISTS shop_products (
    id TEXT PRIMARY KEY,
    name VARCHAR(120) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    category VARCHAR(50) NOT NULL,
    price_cents INTEGER NOT NULL CHECK (price_cents >= 0),
    image_url TEXT NOT NULL DEFAULT '',
    inventory INTEGER NOT NULL DEFAULT 0 CHECK (inventory >= 0),
    featured BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS shop_cart_items (
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id TEXT NOT NULL REFERENCES shop_products(id),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, product_id)
);

CREATE TABLE IF NOT EXISTS shop_orders (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id),
    status VARCHAR(20) NOT NULL CHECK (status IN ('paid', 'cancelled')),
    total_cents INTEGER NOT NULL CHECK (total_cents >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS shop_order_items (
    order_id VARCHAR(36) NOT NULL REFERENCES shop_orders(id) ON DELETE CASCADE,
    product_id TEXT NOT NULL REFERENCES shop_products(id),
    product_name VARCHAR(120) NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_price_cents INTEGER NOT NULL CHECK (unit_price_cents >= 0),
    PRIMARY KEY (order_id, product_id)
);

CREATE TABLE IF NOT EXISTS forum_categories (
    slug VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS forum_threads (
    id VARCHAR(36) PRIMARY KEY,
    category_slug VARCHAR(50) NOT NULL REFERENCES forum_categories(slug),
    user_id VARCHAR(36) NOT NULL REFERENCES users(id),
    title VARCHAR(180) NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS forum_posts (
    id VARCHAR(36) PRIMARY KEY,
    thread_id VARCHAR(36) NOT NULL REFERENCES forum_threads(id) ON DELETE CASCADE,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id),
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_shop_products_category ON shop_products(category);
CREATE INDEX IF NOT EXISTS idx_shop_orders_user_created ON shop_orders(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_forum_threads_activity ON forum_threads(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_forum_threads_category ON forum_threads(category_slug, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_forum_posts_thread_created ON forum_posts(thread_id, created_at);

INSERT INTO shop_products
    (id, name, description, category, price_cents, image_url, inventory, featured)
VALUES
    ('board-fish-58', 'Fish 5''8 Surfboard', 'Fast, playful and made for everyday Mediterranean sessions.', 'Surfboards', 64900, '', 8, TRUE),
    ('wetsuit-3-2', 'Dawn Patrol 3/2mm Wetsuit', 'Flexible warmth for early starts and shoulder-season swells.', 'Wetsuits', 28900, '', 14, TRUE),
    ('leash-comp-6', 'Comp 6'' Leash', 'A lightweight, dependable connection to your board.', 'Accessories', 3900, '', 32, FALSE),
    ('wax-cold', 'Cold Water Wax', 'A grippy, long-lasting bar for cooler water days.', 'Accessories', 700, '', 60, FALSE),
    ('poncho-terry', 'Terry Changing Poncho', 'A soft, roomy layer for changing at the beach.', 'Apparel', 7900, '', 11, TRUE),
    ('fin-set-twin', 'Keel Twin Fin Set', 'Smooth rail-to-rail drive for fish and twin-fin shapes.', 'Fins', 11900, '', 6, FALSE)
ON CONFLICT (id) DO NOTHING;

INSERT INTO forum_categories (slug, name, description, sort_order)
VALUES
    ('local-breaks', 'Local breaks', 'Spot tips, access notes and local knowledge.', 1),
    ('gear-talk', 'Gear talk', 'Boards, wetsuits, fins and equipment advice.', 2),
    ('conditions', 'Conditions', 'Forecast discussion and session planning.', 3),
    ('community', 'Community', 'Meetups, stories and everything in between.', 4)
ON CONFLICT (slug) DO NOTHING;
