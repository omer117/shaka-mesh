# Shaka-Mesh: Distributed Surf & Community Platform

Go microservices architecture built with PostgreSQL/PostGIS, RabbitMQ, and a Next.js client.

> מסמך זה מתאר את המערכת **כפי שהיא ממומשת היום**. דברים מתוכננים שעדיין לא קיימים נמצאים רק בסעיף [Roadmap](#7-roadmap-תכנון-עתידי-לא-ממומש).
> This document describes the system as it is implemented today. Planned work lives only in the Roadmap section.

---

## 1. System Overview (סקירת מערכת)

פלטפורמה קהילתית לגולשים המאפשרת:

- איתור חופים מבוסס מיקום גיאוגרפי (Geo-Spatial queries עם PostGIS) ותנאי ים (גובה גל ורוח) מ-Open-Meteo.
- פורום קהילתי: קטגוריות, דיונים ותגובות.
- חנות ציוד גלישה עם סל קניות ו-checkout מדומה (ללא תשלום) שמתבצע בטרנזקציה אטומית.
- התראות גלים בזמן אמת (Event-Driven) לדפדפן באמצעות RabbitMQ ו-Server-Sent Events (SSE).

---

## 2. Architecture Diagram (דיאגרמת ארכיטקטורה)

כל התקשורת בין הרכיבים היא **HTTP/JSON** (או AMQP מול RabbitMQ). אין gRPC.

```text
            [ Next.js Web Client (Vercel / localhost:3000) ]
                 │                               │
     HTTPS REST + Bearer JWT                     │ SSE  GET /events
                 ▼                               │
   [ Caddy edge ]  (cloud overlay only: 80/443; /events -> notification, rest -> gateway)
                 │                               │
                 ▼                               │
        [ API Gateway :8000 ]                    │
   reverse proxy + JWT (HS256) verification      │
                 │                               │
     HTTP      ┌─┴──────────────┬────────────────┐
               ▼                ▼                ▼
      [ Auth Service ]  [ Beach Service ]  [ Community Service ]
          :8081             :8080              :8083
       users, bcrypt     beaches, PostGIS    shop + cart + orders
       JWT issuing       Open-Meteo refresh  + forum
               │                │                │
               └────────────────┼────────────────┘
                                ▼
                  [ PostgreSQL + PostGIS :5432 ]
                  (one shared DB, local port 5433)

   Beach Service ──publish SwellAlertEvent──► [ RabbitMQ :5672 ]
                                                    │ consume
                                                    ▼
                                       [ Notification Service :8082 ]
                                                    │ SSE
                                                    ▼
                                            Connected browsers
```

הערות:

- הדפדפן מדבר רק עם ה-Gateway (REST) ועם Notification Service (SSE, `GET /events`). בענן שניהם מאחורי Caddy.
- ה-Gateway לא ניגש למסד הנתונים ואינו מפרסם אירועים.
- השירותים כמעט לא קוראים זה לזה. הקשר העיקרי בין שירותים הוא אירוע `SwellAlertEvent` דרך RabbitMQ. החריג היחיד: `auth-service` קורא ל-`GET /api/v1/beaches/{id}` ב-`beach-service` (HTTP פנימי) כדי לאמת `home_beach_id` בעדכון פרופיל.

---

## 3. Microservices Breakdown (חלוקת השירותים)

| Service | Port (container) | Published to host (local compose) | Responsibility |
| ------- | ---------------- | --------------------------------- | -------------- |
| `api-gateway` | 8000 | `8000` | Public HTTP edge |
| `auth-service` | 8081 | no | Users and JWT issuing |
| `beach-service` | 8080 | no | Beaches, geo queries, swell alerts |
| `community-service` | 8083 | no | Shop and forum |
| `notification-service` | 8082 | `8082` (cloud: no, via Caddy) | RabbitMQ consumer and SSE stream |
| `beach-db` (PostGIS 16) | 5432 | `5433` (cloud: no) | Shared PostgreSQL |
| `rabbitmq` | 5672 / 15672 | `5672`, `15672` (cloud: no) | Message broker |
| `edge` (Caddy) | 80 / 443 | cloud overlay only | TLS and routing |

1. **API Gateway (Go, `:8000`):**
   - שער הכניסה היחיד ל-REST. ניתוב (Reverse Proxy) על פי קידומת: `/api/v1/auth/*`, `/api/v1/me*` ו-`/media/avatars/*` ל-auth, `/api/v1/beaches*` ל-beach, `/api/v1/shop*` ו-`/api/v1/forum*` ל-community.
   - מאמת JWT חתום ב-HS256 (סוד משותף `JWT_SECRET`, `JWT_ISSUER`) ומעביר `X-Authenticated-User-ID` ו-`X-Authenticated-User-Email` לשירותים.
   - Public: `/health`, כל `/api/v1/auth/*`, `GET` של חופים, `GET /api/v1/shop/products`, קריאות `GET` של הפורום, ו-`GET /media/avatars/*` (תמונות פרופיל). כל השאר, כולל `/api/v1/me*`, דורש Bearer token.
   - מגדיר CORS לפי `CORS_ALLOWED_ORIGIN`.

2. **Auth Service (Go, `:8081`):**
   - `POST /auth/register`, `POST /auth/login`, `GET /health`. הגישה דרך ה-Gateway היא `/api/v1/auth/...`.
   - סיסמאות נשמרות כ-**bcrypt** (אורך 8-72 בתים). אימייל כפול מחזיר `409`.
   - מנפיק **JWT access token קצר-מועד בלבד** (HS256, ברירת מחדל `JWT_TTL=15m`). אין refresh tokens ואין logout בצד השרת.
   - טבלת `users` ב-PostgreSQL.
   - **פרופיל** (חבילה `internal/profile`): `GET /me`, `PATCH /me` (`display_name`, `bio`, `home_beach_id`), `POST /me/avatar` (multipart). דרך ה-Gateway: `/api/v1/me`, `/api/v1/me/avatar`. הזהות נלקחת רק מ-`X-Authenticated-User-ID` (מה-JWT), אף פעם לא מגוף הבקשה או מה-URL.
   - **תמונות פרופיל:** JPEG/PNG/WebP עד 2MB, הסוג נקבע לפי ה-bytes, התמונה מפוענחת, נחתכת למרכז ל-256x256 ונשמרת מחדש כ-JPEG (מסיר EXIF/מיקום). נשמרת מאחורי ממשק `BlobStore` (כרגע דיסק מקומי, volume בשם `avatar_data`, `AVATAR_STORAGE_DIR`) בשם אקראי, ומוגשת ב-`GET /media/avatars/{id}.jpg` עם `Cache-Control: immutable`. TODO: מימוש OCI Object Storage.

3. **Beach Service (Go + PostGIS, `:8080`):**
   - ניהול חופים: `GET /api/v1/beaches`, `/{id}`, `/nearby?lat&lon&radius_km` (`ST_DWithin`, `ST_Distance`), `/{id}/live-conditions`, ו-`POST /api/v1/beaches`.
   - בעלייה טוענת חופים ישראליים (seed) ומרעננת מ-Open-Meteo גובה גל ורוח בכל `SWELL_REFRESH_INTERVAL` (ברירת מחדל 30 דקות).
   - כאשר גובה גל שנשמר גדול מ-1.5 מטר, מפרסם `SwellAlertEvent` (JSON, persistent) ל-exchange מסוג topic בשם `surf_events` עם routing key `surf.swell.alert`.

4. **Community Service (Go, `:8083`):**
   - **החנות נמצאת כאן** (אין שירות Store & Orders נפרד): מוצרים, סל קניות, הזמנות.
   - `POST /api/v1/shop/checkout` הוא טרנזקציה אטומית אחת: שומרת מלאי, יוצרת הזמנה (סטטוס `paid`, ללא תשלום אמיתי) ומרוקנת את הסל. **Checkout לא מפרסם שום אירוע.**
   - פורום: קטגוריות, דיונים ותגובות. הסכמה וה-seed מוחלים בעלייה.

5. **Notification Service (Go, `:8082`):**
   - צרכן (Consumer) של RabbitMQ: queue עמיד `notifications_swell_queue` הקשור ל-`surf_events` עם `surf.swell.#`. הודעות פגומות נדחות ללא requeue; חיבור מחדש עם backoff.
   - דוחף התראות לדפדפנים דרך **SSE בלבד** (`GET /events`, אירוע בשם `swell-alert`). אין WebSockets. הזרם ציבורי ואינו משחזר אירועים ישנים.
   - צורך רק אירוע אחד: `SwellAlertEvent`.

6. **Web Client (Next.js 16 / React 19 / Tailwind 4 / Leaflet):**
   - `/` חנות, `/forum` פורום, `/conditions` מפת תנאים. מדבר עם ה-Gateway ועם `/events`.

---

## 4. Events (אירועים)

| Event | Producer | Exchange / routing key | Consumer |
| ----- | -------- | ---------------------- | -------- |
| `SwellAlertEvent` | beach-service | `surf_events` / `surf.swell.alert` | notification-service |

זהו האירוע היחיד במערכת. אין `OrderPlaced`.

---

## 5. Design Patterns & Principles (תבניות תכנון)

### ברמת המערכת (System Patterns)

- **Shared database (current state):** Compose מריץ מסד PostgreSQL יחיד (`beach-db`) שכל שירותי ה-DB משתמשים בו. כל שירות מחזיק את הטבלאות שלו (בעלות לוגית), אבל אין הפרדה פיזית.
- **Event-Driven (Pub/Sub):** התראות הגלים עוברות אסינכרונית דרך RabbitMQ, כך ש-beach-service לא תלוי ב-notification-service.
- **API Gateway Pattern:** הסתרת הטופולוגיה הפנימית מהלקוח ואימות טוקן במקום אחד.
- **Stateless JWT auth:** ה-Gateway מאמת חתימה מקומית, בלי קריאה ל-auth-service בכל בקשה.

### ברמת השירות הבודד ב-Go (Internal Code Patterns)

- **Clean / Hexagonal Architecture:**
  - `domain`: מודלים עסקיים טהורים.
  - `repository`: ממשקים לשכבת הנתונים (pgx), מבודדים את ה-SQL מהלוגיקה.
  - `service`: לוגיקה עסקית.
  - `handler`: שכבת HTTP (routing עם `net/http`, JSON, קודי שגיאה).

---

## 6. Technology Stack Summary (סביבה טכנולוגית)

- **Backend:** Go, standard library `net/http`, `pgx`, `amqp091-go`, `golang-jwt`, bcrypt
- **Databases:** PostgreSQL 16 עם PostGIS
- **Message Broker:** RabbitMQ
- **Frontend:** Next.js 16, React 19, TypeScript, Tailwind CSS 4, Leaflet
- **Deployment:** Docker Compose; ענן: Oracle Cloud VM + Caddy (HTTPS) + Vercel (ראו `deployments/REMOTE_DEPLOYMENT.md`)

---

## 7. Roadmap (תכנון עתידי, לא ממומש)

**הפריטים הבאים אינם קיימים בקוד היום.** / None of the following is implemented yet.

- **Refresh tokens עם rotation** ו-`POST /auth/refresh` / `POST /auth/logout`, כולל זיהוי שימוש חוזר.
- **חתימה אסימטרית (RS256/EdDSA)** עם JWKS ו-`kid`, כך שה-Gateway מאמת עם מפתח ציבורי בלבד.
- **Rate limiting** ל-login/register ב-Gateway ומדיניות סיסמאות חזקה יותר.
- **Argon2** כחלופה ל-bcrypt (לא מתוכנן בהכרח).
- **Message reliability:** publisher confirms, DLQ, חיבור מחדש בצד המפרסם.
- **אירוע `OrderPlaced`** והתראות הזמנה (כרגע checkout לא מפרסם אירועים).
- **WebSockets** (כרגע SSE בלבד) ופרופיל משתמש עם תמונה.
- **הפרדת מסדי נתונים** פר שירות, ו-gRPC בין שירותים רק אם יהיה צורך אמיתי.
- **שירות חנות נפרד** אם החנות תגדל מעבר ל-community-service.
