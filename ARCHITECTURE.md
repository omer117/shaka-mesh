# Shaka-Mesh: Distributed Surf & Community Platform

High-Performance Microservices Architecture built with Go, PostgreSQL/PostGIS, RabbitMQ, and React.

---

## 1. System Overview (סקירת מערכת)

פלטפורמה קהילתית מבוזרת לגולשים המאפשרת:

- איתור חופים מבוסס מיקום גיאוגרפי (Geo-Spatial queries), כולל מיון לפי מרחק מהמשתמש.
- תנאי ים חיים (גובה גל, רוח, מחזור גל, כיוון, טמפרטורת מים, גאות ושפל) המתעדכנים מ-Open-Meteo.
- פורום קהילתי: קטגוריות, דיונים ותגובות.
- חנות ציוד גלישה עם סל קניות ו-checkout אטומי (ACID transactions) מדומה, ללא תשלום אמיתי.
- התראות גלים בזמן אמת (Event-Driven Notifications) באמצעות RabbitMQ ו-SSE.

---

## 2. Architecture Diagram (דיאגרמת ארכיטקטורה)

```text
[ Next.js Web Client ]  (Vercel in production, localhost:3000 in dev)
        │                                   ▲
        │ HTTPS / REST                      │ SSE  (GET /events)
        ▼                                   │
[ Caddy Edge ]  (cloud only: TLS, routes /events → Notification, all else → Gateway)
        │                                   │
        ▼                                   │
[ API Gateway (Reverse Proxy + JWT) ]       │
        │                                   │
 ┌──────┼───────────────┬──────────────┐    │
 │ HTTP │ HTTP          │ HTTP         │    │
 ▼      ▼               ▼              │    │
[ Auth ] [ Beach & Geo ] [ Community ]  │    │
(Go)     (Go)            (Go)           │    │
 │        │  ▲            │             │    │
 │        │  └─ Open-Meteo (swell/wind) │    │
 ▼        ▼                ▼            │    │
      PostgreSQL + PostGIS (single instance, per-service tables)
          │
 Beach & Geo ── Publish SwellAlertEvent ──▶ [ RabbitMQ ]
                                                │ Consume
                                                ▼
                                       [ Notification Service ] ──────────┘
                                                (Go, SSE hub)
```

---

## 3. Microservices Breakdown (חלוקת השירותים)

1. **API Gateway Service (`:8000`):**
   - שער הכניסה הציבורי היחיד ל-API; מנתב (Reverse Proxy) לשירותי auth, beach ו-community.
   - אימות JWT (HS256) לפעולות מוגנות; קריאות ציבוריות כוללות חופים, קטלוג החנות וקטגוריות/דיונים בפורום.
   - מעביר את זהות המשתמש המאומת לשירותים בכותרות `X-Authenticated-User-ID` ו-`X-Authenticated-User-Email`.
   - מגדיר CORS לפי `CORS_ALLOWED_ORIGIN`.

2. **Auth & Users Service (Go, `:8081`):**
   - ניהול משתמשים: הרשמה והתחברות (Hashing ב-bcrypt).
   - הנפקת JWT access tokens קצרי חיים (`JWT_TTL`, ברירת מחדל 15 דקות).
   - `JWT_SECRET` (לפחות 32 בתים) ו-`JWT_ISSUER` חייבים להיות זהים ל-Gateway.
   - מסד נתונים: PostgreSQL (טבלת משתמשים).

3. **Beach & Geo Service (Go + PostGIS, `:8080`):**
   - ניהול חופים ונקודות עניין; בעת עלייה מבוצע seed אידמפוטנטי של חופי גלישה מרכזיים בים התיכון בישראל, בנוסף לחופי הדוגמה.
   - שאילתות גיאוגרפיות לחיפוש חופים קרובים בטווח מרחק (`ST_DWithin`, `ST_Distance`).
   - מודל החוף כולל: גובה גל, מהירות רוח, מחזור גל, כיוון swell, טמפרטורת מים, גובה וזמן גאות.
   - **Swell ingestor:** רענון תקופתי (`SWELL_REFRESH_INTERVAL`, ברירת מחדל 30 דקות) של גובה גל (Marine API) ורוח בקשר (Forecast API) מ-Open-Meteo. שגיאות ספק נרשמות ללוג ואינן מוחקות ערכים קיימים. שאר השדות (מחזור, כיוון, טמפרטורה, גאות) הם ערכי דוגמה.
   - `GET /api/v1/beaches/{id}/live-conditions` מפעיל רענון מיידי.
   - פרסום `SwellAlertEvent` ל-exchange `surf_events` (routing key `surf.swell.alert`) כאשר גובה גל שנשמר עולה על 1.5 מטר.
   - נקודת קצה לבדיקות E2E (`POST /api/v1/beaches/_e2e/swell-alert`) מופעלת רק מקומית עם `E2E_ALERT_TEST_TOKEN`, ומנוטרלת תמיד ב-overlay של הענן.

4. **Community Service (Go, `:8083`):**
   - **פורום:** קטגוריות, דיונים ותגובות ב-PostgreSQL; נתוני seed משתמשים בשמות תצוגה מסומנים כדמו ואינם יוצרים חשבונות.
   - **חנות:** קטלוג מוצרים (seed), סל קניות לכל משתמש והזמנות עם checkout מדומה (ההזמנה מסומנת `paid`, ללא גביית תשלום).
   - טרנזקציית checkout אטומית: שמירת מלאי, יצירת הזמנה וריקון הסל.
   - Schema ו-seed נטענים בעלייה באמצעות `migrations/001_community.sql`.

5. **Notification Service (Go, `:8082`):**
   - Consumer של `surf.swell.#` מתור עמיד `notifications_swell_queue`; הודעות פגומות נדחות ללא requeue, וחיבור מחדש ל-RabbitMQ מתבצע עם backoff.
   - מפיץ התראות לדפדפנים דרך Server-Sent Events: `GET /events` (אירוע בשם `swell-alert`). הנקודה ציבורית ואינה משחזרת אירועים ישנים; מנויים איטיים עלולים לדלג על התראות כדי לא לחסום את ה-Consumer.
   - כרגע מטופלים רק אירועי גלים; אירוע `OrderPlaced` טרם מומש.

---

## 4. Deployment (פריסה)

- **Local:** `deployments/docker-compose.yml` מרים את כל השירותים, PostGIS ו-RabbitMQ על הרשת המשותפת `shaka-network`. נדרש `JWT_SECRET` מקובץ `deployments/.env` (לא נכנס ל-git). רק ה-Gateway (`:8000`) וה-Notification Service (`:8082`) חשופים למארח, בנוסף ל-DB (`:5433`) ול-RabbitMQ למטרות פיתוח.
- **Cloud:** `deployments/docker-compose.cloud.yml` הוא overlay על קובץ ה-Compose המקומי: דורש סודות מ-`deployments/.env`, מסיר את פורטי ה-DB, RabbitMQ והשירותים, ומוסיף שירות `edge` (Caddy) שמספק HTTPS אוטומטי ומנתב `/events` ל-Notification Service ושאר הבקשות ל-Gateway. הפריסה על VM של Oracle Cloud (Always Free) והפרונט ב-Vercel; הוראות ב-`deployments/REMOTE_DEPLOYMENT.md`.
- **Images:** כל שירות נבנה מ-`context: ..` עם Dockerfile משלו ומפורסם כ-`omer117/shaka-<service>:latest`.
- **E2E:** `scripts/test-e2e.sh` בודק את זרימת המערכת מקצה לקצה (הרשמה, חופים, קטגוריות פורום והתראת גלים דרך SSE).

---

## 5. Web Client (`web/`)

אפליקציית Next.js / TypeScript (נמצאת כתיקייה רגילה במאגר, לא כ-submodule):

- `/` – חנות הציוד: קטלוג, סל קניות ו-checkout מדומה.
- `/forum` – קטגוריות ודיונים; משתמש מחובר יכול לפתוח נושא ולהגיב.
- `/conditions` – מפת Leaflet עם תנאי ים חיים, חיפוש חופים קרובים לפי מיקום הדפדפן, הצעת חוף חדש והתראות גלים חיות דרך SSE.
- כתובות ה-API וה-SSE נקבעות ב-`NEXT_PUBLIC_API_BASE_URL` ו-`NEXT_PUBLIC_NOTIFICATION_BASE_URL`; ברירת המחדל בפיתוח היא `localhost:8000` / `localhost:8082/events`, ובבניית production היא המארח הפרוס.

---

## 6. Design Patterns & Principles (תבניות תכנון)

### ברמת המערכת המבוזרת (Distributed Patterns)

- **Logical data ownership:** כל שירות הוא הבעלים של הטבלאות שלו. כרגע כל השירותים משתמשים ב-instance אחד של PostgreSQL/PostGIS (`shaka_beach_db`), ללא גישה ישירה לטבלאות של שירות אחר.
- **Event-Driven Architecture (Pub/Sub):** הפרדת תלויות וזמני תגובה מהירים באמצעות הודעות אסינכרוניות ב-RabbitMQ (topic exchange `surf_events`).
- **API Gateway Pattern:** הסתרת הטופולוגיה הפנימית של הרשת מהלקוח; אימות מרוכז של JWT.
- **Edge termination:** בענן, Caddy הוא הרכיב הציבורי היחיד וכל השאר פנימי.

### ברמת השירות הבודד ב-Go (Internal Code Patterns)

- **Clean Architecture / Hexagonal Architecture:**
  - `Domain / Entities`: המודלים העסקיים הטהורים ללא תלות בספריות חיצוניות.
  - `Repository Pattern`: ממשקים (Interfaces) לשכבת הנתונים המבודדים את שאילתות ה-SQL מהלוגיקה העסקית.
  - `Handlers / Transport Layer`: שכבת ה-HTTP (Routing, פירסור JSON, קודי שגיאה).

## 7. Technology Stack Summary (סביבה טכנולוגית)

- **Backend:** Go (Golang)
- **Databases:** PostgreSQL (עם הרחבת PostGIS)
- **Message Broker:** RabbitMQ
- **External data:** Open-Meteo (Marine + Forecast APIs)
- **Frontend:** React (Next.js / TypeScript) + Tailwind CSS + Leaflet Maps
- **Realtime:** Server-Sent Events (SSE)
- **Edge / Hosting:** Caddy, Oracle Cloud VM (backend), Vercel (frontend)
- **Containerization & Dev Environment:** Docker & Docker Compose
