export interface Beach {
  id: string;
  name: string;
  latitude: number;
  longitude: number;
  wave_height: number;
  wind_speed: number;
  wave_period_seconds: number;
  swell_direction_degrees: number;
  water_temperature_celsius: number;
  tide_height_meters: number;
  tide_phase: string;
  description: string;
  created_at: string;
  updated_at: string;
}

export interface AuthSession {
  accessToken: string;
  expiresAt: string;
  email: string;
}

export interface CreateBeachInput {
  name: string;
  latitude: number;
  longitude: number;
  description: string;
}

export interface Product {
  id: string;
  name: string;
  description: string;
  category: string;
  price_cents: number;
  image_url: string;
  inventory: number;
  featured: boolean;
}

export interface CartItem {
  product: Product;
  quantity: number;
  line_cents: number;
}

export interface Cart {
  items: CartItem[];
  total_cents: number;
}

export interface ShopOrder {
  id: string;
  status: string;
  total_cents: number;
  created_at: string;
  items: {
    product_id: string;
    product_name: string;
    quantity: number;
    unit_price_cents: number;
    line_cents: number;
  }[];
}

export interface ForumCategory {
  slug: string;
  name: string;
  description: string;
  thread_count: number;
}

export interface ForumThread {
  id: string;
  category_slug: string;
  title: string;
  body: string;
  author_email: string;
  created_at: string;
  updated_at: string;
  reply_count: number;
}

export interface ForumPost {
  id: string;
  thread_id: string;
  body: string;
  author_email: string;
  created_at: string;
}

export interface ForumThreadDetail {
  thread: ForumThread;
  posts: ForumPost[];
}

const apiBaseUrl =
  process.env.NEXT_PUBLIC_API_BASE_URL ??
  (process.env.NODE_ENV === "production"
    ? "https://151.145.82.171.nip.io"
    : "http://localhost:8000");

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

async function request<T>(
  path: string,
  init?: RequestInit,
  accessToken?: string,
): Promise<T> {
  let response: Response;
  try {
    const headers = new Headers(init?.headers);
    headers.set("Accept", "application/json");
    if (accessToken) headers.set("Authorization", `Bearer ${accessToken}`);

    response = await fetch(`${apiBaseUrl}${path}`, {
      ...init,
      headers,
      cache: "no-store",
    });
  } catch {
    throw new ApiError(0, "Could not connect to the Shaka API gateway.");
  }

  if (!response.ok) {
    let message = `Request failed (${response.status}).`;
    try {
      const body: unknown = await response.json();
      if (
        typeof body === "object" &&
        body !== null &&
        "error" in body &&
        typeof body.error === "string"
      ) {
        message = body.error;
      }
    } catch {
      // Keep the status-based message when the upstream response is not JSON.
    }
    throw new ApiError(response.status, message);
  }

  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

export function registerUser(email: string, password: string): Promise<void> {
  return request<void>("/api/v1/auth/register", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
}

export async function loginUser(
  email: string,
  password: string,
): Promise<AuthSession> {
  const result = await request<{
    access_token: string;
    expires_at: string;
  }>("/api/v1/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  return {
    accessToken: result.access_token,
    expiresAt: result.expires_at,
    email,
  };
}

export function getBeaches(): Promise<Beach[]> {
  return request<Beach[]>("/api/v1/beaches");
}

export function createBeach(
  beach: CreateBeachInput,
  accessToken: string,
): Promise<Beach> {
  return request<Beach>(
    "/api/v1/beaches",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(beach),
    },
    accessToken,
  );
}

export function getProducts(): Promise<Product[]> {
  return request<Product[]>("/api/v1/shop/products");
}

export function getCart(accessToken: string): Promise<Cart> {
  return request<Cart>("/api/v1/shop/cart", undefined, accessToken);
}

export function addCartItem(
  productId: string,
  quantity: number,
  accessToken: string,
): Promise<Cart> {
  return request<Cart>(
    "/api/v1/shop/cart/items",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ product_id: productId, quantity }),
    },
    accessToken,
  );
}

export function removeCartItem(
  productId: string,
  accessToken: string,
): Promise<void> {
  return request<void>(
    `/api/v1/shop/cart/items/${encodeURIComponent(productId)}`,
    { method: "DELETE" },
    accessToken,
  );
}

export function checkout(accessToken: string): Promise<ShopOrder> {
  return request<ShopOrder>(
    "/api/v1/shop/checkout",
    { method: "POST" },
    accessToken,
  );
}

export function getForumCategories(): Promise<ForumCategory[]> {
  return request<ForumCategory[]>("/api/v1/forum/categories");
}

export function getForumThreads(category = ""): Promise<ForumThread[]> {
  const query = category ? `?category=${encodeURIComponent(category)}` : "";
  return request<ForumThread[]>(`/api/v1/forum/threads${query}`);
}

export function getForumThread(threadId: string): Promise<ForumThreadDetail> {
  return request<ForumThreadDetail>(
    `/api/v1/forum/threads/${encodeURIComponent(threadId)}`,
  );
}

export function createForumThread(
  input: { category_slug: string; title: string; body: string },
  accessToken: string,
): Promise<ForumThread> {
  return request<ForumThread>(
    "/api/v1/forum/threads",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    },
    accessToken,
  );
}

export function createForumPost(
  threadId: string,
  body: string,
  accessToken: string,
): Promise<ForumPost> {
  return request<ForumPost>(
    `/api/v1/forum/threads/${encodeURIComponent(threadId)}/posts`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ body }),
    },
    accessToken,
  );
}

export function getNearbyBeaches(
  latitude: number,
  longitude: number,
  radiusKm = 20,
): Promise<Beach[]> {
  const query = new URLSearchParams({
    lat: String(latitude),
    lon: String(longitude),
    radius_km: String(radiusKm),
  });
  return request<Beach[]>(`/api/v1/beaches/nearby?${query}`);
}
