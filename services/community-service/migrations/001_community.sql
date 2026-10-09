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
    user_id VARCHAR(36) REFERENCES users(id),
    author_name VARCHAR(80),
    title VARCHAR(180) NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS forum_posts (
    id VARCHAR(36) PRIMARY KEY,
    thread_id VARCHAR(36) NOT NULL REFERENCES forum_threads(id) ON DELETE CASCADE,
    user_id VARCHAR(36) REFERENCES users(id),
    author_name VARCHAR(80),
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
    ('fin-set-twin', 'Keel Twin Fin Set', 'Smooth rail-to-rail drive for fish and twin-fin shapes.', 'Fins', 11900, '', 6, FALSE),
    ('deck-grip-3pc', 'Three-Piece Deck Grip', 'Lightweight EVA traction with a raised arch and kick tail.', 'Accessories', 5900, '', 18, FALSE),
    ('reef-walker-booties', 'Reef Walker Booties', '3 mm flexible neoprene with a grippy split-toe sole.', 'Wetsuits', 13900, '', 9, FALSE),
    ('coast-cap', 'Coast Canvas Surf Cap', 'Quick-drying canvas, adjustable strap and a leash-ready clip.', 'Apparel', 4900, '', 15, FALSE),
    ('longboard-fin-9', 'Pivot 9-Inch Longboard Fin', 'A dependable single fin for trim, glide and drawn-out turns.', 'Fins', 15900, '', 7, FALSE)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    category = EXCLUDED.category,
    price_cents = EXCLUDED.price_cents,
    image_url = EXCLUDED.image_url,
    featured = EXCLUDED.featured;

INSERT INTO forum_categories (slug, name, description, sort_order)
VALUES
    ('local-breaks', 'Local breaks', 'Spot tips, access notes and local knowledge.', 1),
    ('gear-talk', 'Gear talk', 'Boards, wetsuits, fins and equipment advice.', 2),
    ('conditions', 'Conditions', 'Forecast discussion and session planning.', 3),
    ('community', 'Community', 'Meetups, stories and everything in between.', 4)
ON CONFLICT (slug) DO NOTHING;

ALTER TABLE forum_threads ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE forum_threads ADD COLUMN IF NOT EXISTS author_name VARCHAR(80);
ALTER TABLE forum_posts ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE forum_posts ADD COLUMN IF NOT EXISTS author_name VARCHAR(80);

INSERT INTO forum_threads
    (id, category_slug, user_id, author_name, title, body, created_at, updated_at)
VALUES
    ('demo-local-001', 'local-breaks', NULL, 'Maya · demo', 'First time at Hilton — where should I paddle out?',
     'I usually surf mellow beach breaks and want to try Hilton this weekend. Which part of the reef is friendliest on a small west swell, and is there anything I should know before paddling out?',
     CURRENT_TIMESTAMP - INTERVAL '2 days', CURRENT_TIMESTAMP - INTERVAL '35 minutes'),
    ('demo-gear-001', 'gear-talk', NULL, 'Noam · demo', 'Board volume for a 75 kg beginner?',
     'I am 75 kg, still working on clean pop-ups, and mostly surf waist-high days around Tel Aviv. Would you go for a mid-length first, or stick with a higher-volume shortboard?',
     CURRENT_TIMESTAMP - INTERVAL '1 day', CURRENT_TIMESTAMP - INTERVAL '2 hours'),
    ('demo-conditions-001', 'conditions', NULL, 'Lior · demo', 'Reading the forecast: 8 seconds of period, worth the dawn patrol?',
     'The forecast shows around 0.7 m at 8 seconds with light offshore wind. I am trying to get better at connecting period, direction and local exposure. Would you set an alarm?',
     CURRENT_TIMESTAMP - INTERVAL '18 hours', CURRENT_TIMESTAMP - INTERVAL '4 hours'),
    ('demo-community-001', 'community', NULL, 'Tamar · demo', 'Sunday beach clean-up and coffee after?',
     'A few of us are meeting Sunday morning to pick up the beach before the wind comes up. Bring gloves if you have them; we will bring bags and coffee. Everyone is welcome.',
     CURRENT_TIMESTAMP - INTERVAL '12 hours', CURRENT_TIMESTAMP - INTERVAL '90 minutes')
ON CONFLICT (id) DO NOTHING;

INSERT INTO forum_posts (id, thread_id, user_id, author_name, body, created_at)
VALUES
    ('demo-post-001', 'demo-local-001', NULL, 'Ari · demo',
     'On a smaller swell, give the inside peak a look first and watch a few sets before paddling. The reef gets shallow quickly toward the north end.',
     CURRENT_TIMESTAMP - INTERVAL '30 hours'),
    ('demo-post-002', 'demo-local-001', NULL, 'Maya · demo',
     'Thanks! I will take a look from the beach first and keep plenty of room from the regulars.',
     CURRENT_TIMESTAMP - INTERVAL '28 hours'),
    ('demo-post-003', 'demo-gear-001', NULL, 'Dana · demo',
     'A forgiving mid-length with enough foam under your chest makes those smaller days much more fun. You can size down as your wave count grows.',
     CURRENT_TIMESTAMP - INTERVAL '20 hours'),
    ('demo-post-004', 'demo-gear-001', NULL, 'Noam · demo',
     'That makes sense. I will demo a couple of boards before buying rather than chasing a size chart.',
     CURRENT_TIMESTAMP - INTERVAL '19 hours'),
    ('demo-post-005', 'demo-conditions-001', NULL, 'Eli · demo',
     'Direction matters as much as period for that spot. If the wind really stays offshore, it sounds like a worthwhile check even if it is not a guaranteed session.',
     CURRENT_TIMESTAMP - INTERVAL '8 hours'),
    ('demo-post-006', 'demo-community-001', NULL, 'Roni · demo',
     'Count me in. I can bring a reusable bucket and a few extra gloves.',
     CURRENT_TIMESTAMP - INTERVAL '2 hours')
ON CONFLICT (id) DO NOTHING;
