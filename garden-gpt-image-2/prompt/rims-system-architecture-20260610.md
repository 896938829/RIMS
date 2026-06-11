<!-- SPDX-License-Identifier: AGPL-3.0-or-later -->
<!-- Copyright (c) 2026 ShangBin Wang -->

Create a polished technical system architecture diagram for "RIMS - Retail Inventory Management System".

Format:
- 16:9 horizontal canvas
- Dark engineering style: deep slate #0F172A background with subtle 1px grid lines #1E293B at 32px spacing
- Use JetBrains Mono / SF Mono style typography
- Clean rounded rectangles, radius 8px, semi-transparent fills, thin colored borders
- Orthogonal arrows that do not cross nodes
- Include a compact legend in the bottom-right
- This is a PNG-style documentation illustration, not editable SVG

Title:
- "RIMS Backend Architecture"
- Subtitle: "Gin + GORM + PostgreSQL · modular retail inventory system · 2026"

Regions and nodes:

1. Client / Public Edge region, cyan border:
- Admin / Staff Client
- Swagger UI
- Public Uploads /uploads/*

2. HTTP API Layer region, blue border:
- Gin Router / API v1
- Middleware Chain: Recovery, RequestID, Logger, CORS, JWTAuth, WarehouseScope
- Permission Middleware
- Idempotency Middleware

3. Business Modules region, emerald border:
- User & RBAC
- Warehouse
- Product & Inventory
- Document Engine
- Report Queries
- File Attachment
- Audit Log

4. Shared Infrastructure region, amber border:
- Config / Viper
- Auth / JWT
- Tx Runner / Context Propagation
- Migration Runner
- Cleanup Command

5. Data Plane region, violet border:
- PostgreSQL 16
- Local File Storage

Main data flows:
- Admin / Staff Client -> Gin Router / API v1: REST + JSON
- Swagger UI -> Gin Router / API v1: API docs
- Gin Router / API v1 -> Middleware Chain
- Middleware Chain -> Permission Middleware: protected admin routes
- Middleware Chain -> WarehouseScope: warehouse-scoped routes
- Middleware Chain -> Idempotency Middleware: unsafe write endpoints
- Middleware Chain -> Business Modules
- Business Modules -> Tx Runner / Context Propagation: transactional writes
- Business Modules -> PostgreSQL 16: GORM repositories
- File Attachment -> Local File Storage: upload/download object bytes
- File Attachment -> PostgreSQL 16: metadata
- Public Uploads /uploads/* -> Local File Storage: static public product images
- Document Engine -> Product & Inventory: stock movements with row locks
- Business Modules -> Audit Log: append-only audit events
- Migration Runner -> PostgreSQL 16: versioned SQL migrations
- Cleanup Command -> PostgreSQL 16: expired idempotency/audit metadata cleanup
- Cleanup Command -> Local File Storage: delete orphaned retained objects

Visual semantics:
- Cyan = Client / Edge
- Blue = HTTP / API gateway
- Rose = Security / Auth / Permission / Audit
- Emerald = Business modules
- Amber = Shared infrastructure / operations
- Violet = Persistence / storage

Constraints:
- Keep total visual nodes readable; group middleware as one stack node and modules as compact subnodes.
- Do not use real brand logos or emojis.
- Use accurate labels exactly as above where possible.
- Use concise labels; avoid long paragraphs in nodes.
- No 3D, no glassmorphism, no decorative blobs.
