# Shaka-Mesh: Distributed Surf & Community Platform

High-Performance Microservices Architecture built with Go, PostgreSQL/PostGIS, RabbitMQ, and React.

---

## 1. System Overview (סקירת מערכת)

פלטפורמה קהילתית מבוזרת לגולשים המאפשרת:

- איתור חופים מבוסס מיקום גיאוגרפי (Geo-Spatial queries).
- שיתוף פוסטים, דיווחי גלים ועדכונים קהילתיים בזמן אמת.
- חנות ציוד גלישה עם ניהול הזמנות אטומי (ACID transactions).
- מערכת התראות מבוססת אירועים (Event-Driven Notifications) לגולשים על תנאי ים והזמנות.

---

## 2. Architecture Diagram (דיאגרמת ארכיטקטורה)

\`\`\`text
[ React / Next.js Web Client ]
│
│ HTTPS / REST / WebSockets
▼
[ API Gateway (Reverse Proxy) ]
│
┌─────────────────────┼─────────────────────┬─────────────────────┐
│ (HTTP/gRPC) │ (HTTP/gRPC) │ (HTTP/gRPC) │ (HTTP/gRPC)
▼ ▼ ▼ ▼
[ Auth & Users ] [ Beach & Geo ] [ Community ] [ Store & Orders ]
(Go Service) (Go Service) (Go Service) (Go Service)
│ │ │ │
▼ ▼ ▼ ▼
(PostgreSQL) (PostgreSQL + PostGIS) (PostgreSQL) (PostgreSQL)
│ │ │
└──────────────┬──────┴──────────────────────┘
│ Publish Events
▼
[ RabbitMQ Message Broker ]
│
│ SwellAlertEvent / OrderPlacedEvent
▼ (Subscribe / Consume)
[ Notification Service ]
(Go Service)
│
▼ WebSockets / SSE
[ Connected Users / Browsers ]
\`\`\`

---

## 3. Microservices Breakdown (חלוקת השירותים)

1. **API Gateway Service:**
   - שער הכניסה הראשי לכל הפניות מה-Frontend.
   - אימות ראשוני של טוקנים (JWT Verification).
   - ניתוב (Reverse Proxy) לשירותים הפנימיים.

2. **Auth & Users Service (Go):**
   - ניהול משתמשים: הרשמה, התחברות (Hashing ב-Argon2/Bcrypt).
   - הנפקת JWT Tokens ו-Refresh Tokens.
   - מסד נתונים: PostgreSQL (טבלת משתמשים ופרופילים).

3. **Beach & Geo Service (Go + PostGIS):**
   - ניהול נתוני חופים ונקודות עניין (Latitude, Longitude).
   - שאילתות גיאוגרפיות לחיפוש חופים קרובים בטווח מרחק מוגדר (`ST_DWithin`, `ST_Distance`).
   - פרסום אירוע `SwellAlertTriggered` כאשר תנאי הים בחוף עולים על רף מוגדר.

4. **Community Service (Go):**
   - פורומים, פוסטים, דיווחי גלים ותגובות.
   - מסד נתונים: PostgreSQL.

5. **Store & Orders Service (Go):**
   - קטלוג מוצרים, ניהול מלאי והזמנות.
   - טרנזקציות ACID ב-PostgreSQL כדי למנוע מכירת יתר.
   - פרסום אירוע `OrderPlaced` בעת סיום הזמנה.

6. **Notification Service (Go):**
   - מאזין (Consumer) לאירועים מ-RabbitMQ (`OrderPlaced`, `SwellAlertTriggered`).
   - דוחף התראות למשתמשים בזמן אמת דרך WebSockets או Server-Sent Events (SSE).

7. **Auth Service and API Gateway (Go):**
   - Auth service stores users, hashes passwords with bcrypt, and issues short-lived JWT access tokens.
   - API gateway is the public HTTP edge, routes auth/beach requests, and verifies JWTs before protected beach operations.
   - Compose connects services on the shared `shaka-network`; backend ports are not published to the host.

---

## 4. Design Patterns & Principles (תבניות תכנון)

### ברמת המערכת המבוזרת (Distributed Patterns)

- **Database-per-Service:** כל מיקרוסרביס הוא בעל הנתונים היחיד של ה-Schema שלו. אין גישה ישירה של שירות למסד של שירות אחר.
- **Event-Driven Architecture (Pub/Sub):** הפרדת תלויות וזמני תגובה מהירים באמצעות הודעות אסינכרוניות ב-RabbitMQ.
- **API Gateway Pattern:** הסתרת הטופולוגיה הפנימית של הרשת מהלקוח.

### ברמת השירות הבודד ב-Go (Internal Code Patterns)

- **Clean Architecture / Hexagonal Architecture:**
  - `Domain / Entities`: המודלים העסקיים הטהורים ללא תלות בספריות חיצוניות.
  - `Repository Pattern`: ממשקים (Interfaces) לשכבת הנתונים המבודדים את שאילתות ה-SQL מהלוגיקה העסקית.
  - `Handlers / Transport Layer`: שכבת ה-HTTP (Routing, פירסור JSON, קודי שגיאה).

## 5. Technology Stack Summary (סביבה טכנולוגית)

- **Backend:** Go (Golang)
- **Databases:** PostgreSQL (עם הרחבת PostGIS)
- **Message Broker:** RabbitMQ
- **Frontend:** React (Next.js / TypeScript) + Tailwind CSS + Leaflet Maps
- **Containerization & Dev Environment:** Docker & Docker Compose
