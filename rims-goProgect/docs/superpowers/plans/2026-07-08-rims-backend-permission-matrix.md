# RIMS Backend Permission Matrix

| Method | Path | Module | Auth Required | Permission Required | Public Reason |
| --- | --- | --- | --- | --- | --- |
| GET | /healthz | system | No | None | Health probe |
| GET | /swagger/*any | system | No | None | API documentation in dev environment |
| GET | /uploads/*filepath | file | No | None | Public local objects such as product images |
| POST | /api/v1/auth/login | user | No | None | Login endpoint |
| POST | /api/v1/auth/register | user | No | None | Self-service ordinary user registration |
| POST | /api/v1/users | user | Yes | user:create | User management |
| GET | /api/v1/users | user | Yes | user:list | User management |
| GET | /api/v1/users/me | user | Yes | None | Current authenticated user's own profile |
| PUT | /api/v1/users/me/password | user | Yes | None | Current authenticated user's own password change |
| GET | /api/v1/users/:id | user | Yes | user:read | User management |
| PUT | /api/v1/users/:id | user | Yes | user:update | User management |
| DELETE | /api/v1/users/:id | user | Yes | user:delete | User management |
| PUT | /api/v1/users/:id/password | user | Yes | user:reset_password | User management |
| POST | /api/v1/roles | user | Yes | role:create | Role management |
| GET | /api/v1/roles | user | Yes | role:list | Role management metadata |
| GET | /api/v1/roles/:id | user | Yes | role:read | Role management metadata |
| PUT | /api/v1/roles/:id | user | Yes | role:update | Role management |
| DELETE | /api/v1/roles/:id | user | Yes | role:delete | Role management |
| PUT | /api/v1/roles/:id/permissions | user | Yes | role:assign_permissions | Permission assignment |
| GET | /api/v1/permissions | user | Yes | permission:list | Permission management metadata |
| POST | /api/v1/warehouses | warehouse | Yes | warehouse:create | Warehouse management |
| GET | /api/v1/warehouses | warehouse | Yes | None | Service scopes results to current user's accessible warehouses; admin sees all |
| GET | /api/v1/warehouses/:id | warehouse | Yes | warehouse:read | Warehouse management detail |
| PUT | /api/v1/warehouses/:id | warehouse | Yes | warehouse:update | Warehouse management |
| DELETE | /api/v1/warehouses/:id | warehouse | Yes | warehouse:delete | Warehouse management |
| POST | /api/v1/warehouses/:id/users | warehouse | Yes | warehouse:bind_user | Warehouse-user binding management |
| DELETE | /api/v1/warehouses/:id/users/:userId | warehouse | Yes | warehouse:unbind_user | Warehouse-user binding management |
| GET | /api/v1/warehouses/:id/users | warehouse | Yes | warehouse:list_users | Warehouse-user binding management |
| GET | /api/v1/users/me/warehouses | warehouse | Yes | None | Current authenticated user's own warehouse list |
| PUT | /api/v1/users/me/warehouses/default | warehouse | Yes | None | Current authenticated user's own default warehouse selection |
| PUT | /api/v1/users/me/warehouses/current | warehouse | Yes | None | Current authenticated user's own current warehouse selection |
| POST | /api/v1/products | product | Yes | product:create | Product management |
| GET | /api/v1/products | product | Yes | None | Product catalog lookup for authenticated operators |
| GET | /api/v1/products/barcode/:barcode | product | Yes | None | Product catalog lookup during sales/inbound workflows |
| GET | /api/v1/products/:id | product | Yes | None | Product catalog detail for authenticated operators |
| PUT | /api/v1/products/:id | product | Yes | product:update | Product management |
| DELETE | /api/v1/products/:id | product | Yes | product:delete | Product management |
| GET | /api/v1/inventory | product | Yes | None | Warehouse-scoped inventory workbench |
| GET | /api/v1/inventory/alerts | product | Yes | None | Warehouse-scoped inventory alerts |
| GET | /api/v1/inventory/:id | product | Yes | None | Warehouse-scoped inventory detail |
| PUT | /api/v1/inventory/:id | product | Yes | inventory:update | Inventory settings management |
| POST | /api/v1/non-std-inventory | product | Yes | non_std:create | Non-standard inventory management |
| GET | /api/v1/non-std-inventory | product | Yes | non_std:read | Non-standard inventory management |
| GET | /api/v1/non-std-inventory/:id | product | Yes | non_std:read | Non-standard inventory management |
| PUT | /api/v1/non-std-inventory/:id | product | Yes | non_std:update | Non-standard inventory management |
| DELETE | /api/v1/non-std-inventory/:id | product | Yes | non_std:delete | Non-standard inventory management |
| POST | /api/v1/non-std-inventory/:id/convert | product | Yes | non_std:convert | Non-standard inventory conversion |
| POST | /api/v1/documents | document | Yes | None | Warehouse-scoped business document creation |
| GET | /api/v1/documents | document | Yes | None | Warehouse-scoped business document list |
| GET | /api/v1/documents/:id | document | Yes | None | Warehouse-scoped business document detail |
| POST | /api/v1/documents/:id/complete | document | Yes | None | Warehouse-scoped business document completion |
| POST | /api/v1/documents/:id/confirm | document | Yes | None | Warehouse-scoped stocktake confirmation |
| POST | /api/v1/documents/:id/settle | document | Yes | None | Warehouse-scoped stocktake settlement |
| GET | /api/v1/transactions | document | Yes | None | Warehouse-scoped inventory transaction history |
| GET | /api/v1/reports/sales/stats | report | Yes | None | Warehouse-scoped analytics; cost/profit fields are admin-gated in handler/service response shaping |
| GET | /api/v1/reports/sales/trend | report | Yes | None | Warehouse-scoped analytics |
| GET | /api/v1/reports/sales/ranking | report | Yes | None | Warehouse-scoped analytics |
| GET | /api/v1/reports/inventory/overview | report | Yes | None | Warehouse-scoped analytics; stock-value fields are admin-gated in handler/service response shaping |
| GET | /api/v1/reports/inventory/turnover | report | Yes | None | Warehouse-scoped analytics |
| GET | /api/v1/reports/inventory/slow-moving | report | Yes | None | Warehouse-scoped analytics |
| POST | /api/v1/files/upload | file | Yes | None | Upload ownership and business ACL are enforced in file service |
| GET | /api/v1/files | file | Yes | None | File list is scoped by uploader/business ACL in file service |
| GET | /api/v1/files/:id | file | Yes | None | File metadata read is scoped by uploader/business ACL in file service |
| GET | /api/v1/files/:id/download | file | Yes | None | Private file download is proxied through uploader/business ACL in file service |
| DELETE | /api/v1/files/:id | file | Yes | None | File delete requires uploader or admin in file service |
| GET | /api/v1/audit/logs | audit | Yes | audit:read | Audit log read surface |
| GET | /api/v1/audit/logs/:id | audit | Yes | audit:read | Audit log read surface |
