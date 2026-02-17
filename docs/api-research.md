# API Research: orderfoodonline.deno.dev

Comprehensive behavioral analysis of the live API at `https://orderfoodonline.deno.dev/api`.

**Date:** 2026-02-17
**Base URL:** `https://orderfoodonline.deno.dev`
**API prefix:** `/api`

---

## Table of Contents

1. [General Observations](#general-observations)
2. [GET /api/product](#get-apiproduct)
3. [GET /api/product/{productId}](#get-apiproductproductid)
4. [POST /api/order](#post-apiorder)
5. [Coupon Code Behavior](#coupon-code-behavior)
6. [Spec vs Reality Discrepancies](#spec-vs-reality-discrepancies)
7. [Product Catalog Reference](#product-catalog-reference)

---

## General Observations

### Server Identity
- **Server header:** `deno/gcp-asia-southeast1`
- **Protocol:** HTTP/2
- **Platform:** Deno Deploy

### Content-Type
- All JSON responses use `content-type: application/json; charset=UTF-8`
- JSON parse errors return `content-type: text/plain; charset=UTF-8`

### CORS
- **No CORS headers are returned.** Not on regular requests, not on preflight OPTIONS requests.
- OPTIONS requests return `200` with `Allow` header listing permitted methods, but no `Access-Control-*` headers.
- This means browser-based clients on different origins cannot call this API directly.

### Rate Limiting
- **No rate-limiting headers observed** (no `X-RateLimit-*`, `Retry-After`, etc.)

### Authentication
- **The `api_key` header is completely ignored.** The API does not validate authentication at all:
  - Missing `api_key` header: request succeeds (200)
  - Wrong `api_key` value (e.g., `wrongkey`): request succeeds (200)
  - Empty `api_key` value: request succeeds (200)
- The spec documents `api_key` security on POST /api/order, but it is not enforced.

### HTTP Method Handling
- Wrong HTTP methods return `405 Method Not Allowed` with an `Allow` header:
  - `GET /api/order` -> `405`, `Allow: POST`
  - `DELETE /api/product/1` -> `405`, `Allow: HEAD, GET`
  - `PUT /api/order` -> `405`, `Allow: POST`
- **HEAD requests are supported** on GET endpoints (product list and single product).

### 404 Behavior
- All 404 responses return **empty body** with `content-length: 0`.
- No JSON error object is returned, contradicting the spec which defines an Error schema for 404.

### Undocumented Endpoints
All of these return `404` with empty body:
- `/api/health`
- `/api/status`
- `/livez`
- `/readyz`
- `/healthz`
- `/api` (root)
- `/` (root)
- `/api/products` (plural)
- `/api/orders` (plural)

### Static Assets
- Image files are served at `/public/images/...` with:
  - `content-type: image/jpeg`
  - `cache-control: max-age=0`
  - `etag` and `last-modified` headers present

---

## GET /api/product

### Normal Request

```bash
curl -s "https://orderfoodonline.deno.dev/api/product"
```

**Status:** `200 OK`
**Response:** JSON array of 9 product objects.

```json
[
  {
    "id": "1",
    "image": {
      "thumbnail": "https://orderfoodonline.deno.dev/public/images/image-waffle-thumbnail.jpg",
      "mobile": "https://orderfoodonline.deno.dev/public/images/image-waffle-mobile.jpg",
      "tablet": "https://orderfoodonline.deno.dev/public/images/image-waffle-tablet.jpg",
      "desktop": "https://orderfoodonline.deno.dev/public/images/image-waffle-desktop.jpg"
    },
    "name": "Waffle with Berries",
    "category": "Waffle",
    "price": 6.5
  }
]
```

### Key observations:
- Returns exactly **9 products** (IDs "1" through "9").
- Product field order: `id`, `image`, `name`, `category`, `price`.
- The `image` field is a nested object with `thumbnail`, `mobile`, `tablet`, `desktop` URLs.
- All product IDs are **string type** ("1", "2", etc.).
- Prices are **numbers** (not strings). Some are integers (7, 8, 4, 5), some are decimals (6.5, 5.5, 4.5).

### Accept Header Variations
- `Accept: text/xml` -> Still returns JSON (200). Accept header is ignored.
- `Accept: text/plain` -> Still returns JSON (200). Accept header is ignored.

### Trailing Slash
- `GET /api/product/` -> Returns the full product list (same as `/api/product`). The trailing slash is treated as the list endpoint, NOT as an empty productId.

### Response Headers (typical)
```
content-type: application/json; charset=UTF-8
vary: Accept-Encoding
content-length: 3835
server: deno/gcp-asia-southeast1
```

---

## GET /api/product/{productId}

### Valid IDs (1-9)

```bash
curl -s "https://orderfoodonline.deno.dev/api/product/1"
```

**Status:** `200 OK`
**Response:** Single product object (same structure as in the list).

```json
{
  "id": "1",
  "image": { ... },
  "name": "Waffle with Berries",
  "category": "Waffle",
  "price": 6.5
}
```

### Invalid/Non-existent IDs

| Input | Status | Body | Content-Type |
|-------|--------|------|--------------|
| `0` | 404 | (empty) | (none) |
| `-1` | 404 | (empty) | (none) |
| `10` | 404 | (empty) | (none) |
| `999` | 404 | (empty) | (none) |
| `abc` | 404 | (empty) | (none) |

### Key observations:
- **All non-existent IDs return 404 with empty body.** There is no distinction between "invalid ID" (400) and "not found" (404) as the spec suggests.
- The spec defines both `400 Invalid ID supplied` and `404 Product not found` error responses with JSON Error schema, but in practice **only 404 is returned** and **no JSON body** is included.
- Non-numeric strings like "abc" also get 404 (not 400).

---

## POST /api/order

### Request Format

```bash
curl -s -X POST "https://orderfoodonline.deno.dev/api/order" \
  -H "Content-Type: application/json" \
  -H "api_key: apitest" \
  -d '{"items":[{"productId":"1","quantity":1}]}'
```

### Successful Order Response

**Status:** `200 OK`

```json
{
  "items": [
    {"productId": "1", "quantity": 1}
  ],
  "id": "1643dcc7-024f-4ed5-a951-3437841804bf",
  "products": [
    {
      "id": "1",
      "image": { ... },
      "name": "Waffle with Berries",
      "category": "Waffle",
      "price": 6.5
    }
  ]
}
```

### Response Structure Analysis

The successful order response includes:
- `items` - The items array from the request, echoed back exactly as sent
- `id` - A UUID v4 generated by the server
- `products` - Array of full Product objects matching the productIds in items

**MISSING from response (vs spec):**
- `total` - The spec defines this field but it is **never returned**
- `discounts` - The spec defines this field but it is **never returned**

The `products` array contains one entry per item in the `items` array (in the same order). If the same productId appears twice in items, the same product appears twice in products.

### Multi-Item Order

```bash
curl -s -X POST "https://orderfoodonline.deno.dev/api/order" \
  -H "Content-Type: application/json" \
  -H "api_key: apitest" \
  -d '{"items":[{"productId":"1","quantity":2},{"productId":"3","quantity":1}]}'
```

**Status:** `200 OK`
**Response:** items array echoed back, products array contains product 1 and product 3.

### All 9 Products Order

Ordering all 9 products at once works fine. Response includes all 9 items and all 9 product objects.

---

### Error Scenarios

#### No Request Body

```bash
curl -s -X POST "https://orderfoodonline.deno.dev/api/order" \
  -H "Content-Type: application/json" -H "api_key: apitest"
```

**Status:** `400`
**Content-Type:** `text/plain; charset=UTF-8`
**Body:** `Unexpected end of JSON input`

Note: This is a **plain text** error, not JSON.

#### Malformed JSON Body

```bash
curl -s -X POST ... -d 'not json'
```

**Status:** `400`
**Content-Type:** `text/plain; charset=UTF-8`
**Body:** `Unexpected token 'o', "not json" is not valid JSON`

Note: Also **plain text**, not the JSON Error schema from the spec.

#### Empty Object Body `{}`

```bash
curl -s -X POST ... -d '{}'
```

**Status:** `200` (NOT 400!)
**Content-Type:** `application/json; charset=UTF-8`
**Body:**
```json
{"code": "validation", "message": "at least one item is required"}
```

**IMPORTANT:** This validation error returns HTTP 200, not 400 or 422.

#### Empty Items Array `{"items":[]}`

**Status:** `200` (NOT 400!)
**Body:**
```json
{"code": "validation", "message": "at least one item is required"}
```

Same as empty object -- HTTP 200 with validation error in body.

#### Null Items `{"items":null}`

**Status:** `200`
**Body:**
```json
{"code": "validation", "message": "at least one item is required"}
```

#### Empty Item Object `{"items":[{}]}`

**Status:** `400`
**Body:**
```json
{"code": "validation", "message": "productId for item is required"}
```

#### Missing productId `{"items":[{"quantity":1}]}`

**Status:** `400`
**Body:**
```json
{"code": "validation", "message": "productId for item is required"}
```

#### Empty productId `{"items":[{"productId":"","quantity":1}]}`

**Status:** `400`
**Body:**
```json
{"code": "validation", "message": "productId for item is required"}
```

#### Missing Quantity `{"items":[{"productId":"1"}]}`

**Status:** `400`
**Body:**
```json
{"code": "validation", "message": "item quantity cannot be less than zero"}
```

Note: Missing quantity is treated as "less than zero", not as a distinct "required field" error.

#### Null Quantity `{"items":[{"productId":"1","quantity":null}]}`

**Status:** `400`
**Body:**
```json
{"code": "validation", "message": "item quantity cannot be less than zero"}
```

#### Quantity = 0

**Status:** `400`
**Body:**
```json
{"code": "validation", "message": "item quantity cannot be less than zero"}
```

Note: Zero is treated as "less than zero" even though it is not. The error message is misleading.

#### Negative Quantity (-1)

**Status:** `200` (ACCEPTED!)
**Body:** Normal successful order response with quantity -1 preserved.

**BUG:** Negative quantity is accepted by the server. Only quantity 0 is rejected (as "less than zero"). Actual negative values pass validation.

#### Float Quantity (1.5)

**Status:** `200` (ACCEPTED!)
**Body:** Normal successful order response with quantity 1.5 preserved.

**BUG:** Float quantities are accepted despite the spec declaring `type: integer`.

#### String Quantity ("abc")

**Status:** `200` (ACCEPTED!)
**Body:** Normal successful order response with quantity "abc" preserved.

**BUG:** String quantities are accepted despite the spec declaring `type: integer`.

#### Very Large Quantity (999999)

**Status:** `200` (ACCEPTED!)
No upper bound validation.

#### Non-Existent productId ("999")

**Status:** `422`
**Body:**
```json
{"code": "constraint", "message": "invalid product specified"}
```

#### Invalid productId Format ("abc")

**Status:** `422`
**Body:**
```json
{"code": "constraint", "message": "invalid product specified"}
```

Same error as non-existent numeric ID.

#### Numeric productId (integer 1 instead of string "1")

**Status:** `200` (ACCEPTED!)
The server coerces numeric productId to find the product. Response echoes back the numeric type as-is.

#### Mixed Valid and Invalid productIds

```bash
-d '{"items":[{"productId":"1","quantity":1},{"productId":"999","quantity":1}]}'
```

**Status:** `422`
**Body:**
```json
{"code": "constraint", "message": "invalid product specified"}
```

The entire order is rejected if any productId is invalid.

#### Mixed Valid Item and quantity=0

```bash
-d '{"items":[{"productId":"1","quantity":1},{"productId":"2","quantity":0}]}'
```

**Status:** `400`
**Body:**
```json
{"code": "validation", "message": "item quantity cannot be less than zero"}
```

Validation errors take priority -- the entire order is rejected.

#### Duplicate productIds

```bash
-d '{"items":[{"productId":"1","quantity":1},{"productId":"1","quantity":2}]}'
```

**Status:** `200` (ACCEPTED!)
**Body:** The products array contains the same product TWICE (one per item entry). No deduplication.

```json
{
  "items": [
    {"productId": "1", "quantity": 1},
    {"productId": "1", "quantity": 2}
  ],
  "products": [
    {"id": "1", "name": "Waffle with Berries", "price": 6.5, ...},
    {"id": "1", "name": "Waffle with Berries", "price": 6.5, ...}
  ]
}
```

#### Extra Fields in Request Body

```bash
-d '{"items":[{"productId":"1","quantity":1}],"extraField":"test"}'
```

**Status:** `200` (ACCEPTED!)
Extra fields are **preserved and echoed back** in the response:
```json
{"items":[...],"extraField":"test","id":"...","products":[...]}
```

#### Extra Fields in Item Objects

```bash
-d '{"items":[{"productId":"1","quantity":1,"extraItemField":"test"}]}'
```

**Status:** `200` (ACCEPTED!)
Extra fields in item objects are also preserved:
```json
{"items":[{"productId":"1","quantity":1,"extraItemField":"test"}], ...}
```

#### Without Content-Type Header

**Status:** `200` (ACCEPTED!)
The server parses JSON regardless of Content-Type header. It does not require `Content-Type: application/json`.

#### Form-Encoded Content-Type

**Status:** `400`
**Body:** `Unexpected token 'i', "items[0][p"... is not valid JSON`

The server always tries to parse as JSON regardless of Content-Type.

---

## Coupon Code Behavior

### Summary

**The API does not validate coupon codes in any way.** Any string value for `couponCode` is accepted and echoed back in the response. The API never calculates totals or discounts -- it is the **client's responsibility** to compute these.

### Test Results

| Coupon Code | Status | Result |
|-------------|--------|--------|
| `HAPPYHOURS` | 200 | Accepted, echoed back |
| `HAPPYHRS` | 200 | Accepted, echoed back |
| `BUYGETONE` (1 item) | 200 | Accepted, echoed back |
| `BUYGETONE` (multiple items) | 200 | Accepted, echoed back |
| `FIFTYOFF` | 200 | Accepted, echoed back |
| `SUPER100` | 200 | Accepted, echoed back |
| `NONEXISTENT` | 200 | Accepted, echoed back |
| `happyhours` (lowercase) | 200 | Accepted, echoed back |
| `""` (empty string) | 200 | Accepted, echoed back as `""` |

### Key Insight

The coupon code is simply stored as a pass-through field on the order. No discount logic exists server-side. The response **never** includes `total` or `discounts` fields, regardless of coupon code.

This means:
- Coupon validation must be done client-side (or by our implementation)
- Discount calculation must be done client-side (or by our implementation)
- The coupon code names (`HAPPYHOURS`, `HAPPYHRS`, `BUYGETONE`, `FIFTYOFF`) suggest intended discount types, but the reference API does not implement them

---

## Spec vs Reality Discrepancies

### Error Schema Mismatch

**Spec says:**
```yaml
Error:
  type: object
  required: [code, message]
  properties:
    code:
      type: integer
      format: int32
    message:
      type: string
```

**Reality:**
```json
{"code": "validation", "message": "..."}
{"code": "constraint", "message": "..."}
```

The `code` field is a **string** (e.g., "validation", "constraint"), NOT an integer as the spec declares.

### Missing Order Response Fields

**Spec says** Order schema includes:
- `id` (string) -- present
- `total` (number) -- **MISSING**
- `discounts` (number) -- **MISSING**
- `items` (array of OrderItem) -- present
- `products` (array of Product) -- present

### Authentication Not Enforced

**Spec says:** POST /api/order requires `api_key` security scheme.
**Reality:** The `api_key` header is completely ignored. All requests succeed regardless.

### Error Status Codes Inconsistent

| Scenario | Spec Says | Reality |
|----------|-----------|---------|
| Empty items / missing items | 400 | **200** |
| Null items | 400 | **200** |
| productId missing | 400 | 400 (correct) |
| Quantity = 0 | 400 | 400 (correct) |
| Quantity = -1 | 400 | **200** (accepted!) |
| Non-existent product | 422 | 422 (correct) |
| Product not found (GET) | 404 with JSON | **404 empty body** |
| Invalid ID (GET) | 400 with JSON | **404 empty body** |

### No 400 vs 404 Distinction for GET /api/product/{id}

The spec defines separate 400 (Invalid ID) and 404 (Not Found) responses. In reality, ALL invalid/missing products return 404 with an empty body.

### OPTIONS Responses

OPTIONS returns 200 with `Allow` header but no CORS headers. This is not standard preflight behavior -- a real CORS preflight expects `Access-Control-Allow-*` headers.

---

## Product Catalog Reference

| ID | Name | Category | Price |
|----|------|----------|-------|
| 1 | Waffle with Berries | Waffle | 6.50 |
| 2 | Vanilla Bean Creme Brulee | Creme Brulee | 7.00 |
| 3 | Macaron Mix of Five | Macaron | 8.00 |
| 4 | Classic Tiramisu | Tiramisu | 5.50 |
| 5 | Pistachio Baklava | Baklava | 4.00 |
| 6 | Lemon Meringue Pie | Pie | 5.00 |
| 7 | Red Velvet Cake | Cake | 4.50 |
| 8 | Salted Caramel Brownie | Brownie | 4.50 |
| 9 | Vanilla Panna Cotta | Panna Cotta | 6.50 |

**Total products:** 9
**Price range:** 4.00 - 8.00
**All IDs are strings** ("1" through "9").

### Image URL Pattern

All images follow the pattern:
```
https://orderfoodonline.deno.dev/public/images/image-{slug}-{size}.jpg
```

Where `{size}` is one of: `thumbnail`, `mobile`, `tablet`, `desktop`.

Image slugs by product:
- waffle
- creme-brulee
- macaron
- tiramisu
- baklava
- meringue
- cake
- brownie
- panna-cotta

---

## Validation Priority Order

Based on testing, the server validates in this order:

1. **JSON parse** -- if body is not valid JSON -> 400 plain text error
2. **Items existence** -- if items is missing/null/empty -> 200 with validation error (inconsistent!)
3. **Item productId required** -- if any item lacks productId or has empty productId -> 400 JSON error
4. **Item quantity >= 0** -- if any item has quantity 0, null, or missing -> 400 JSON error (but -1 passes!)
5. **Product existence** -- if any productId does not match a real product -> 422 JSON error
6. **Success** -- order created with UUID, items echoed, products resolved

---

## Error Response Formats

There are exactly 3 error response formats used:

### 1. Plain text JSON parse error (400)
```
Content-Type: text/plain; charset=UTF-8
Body: Unexpected end of JSON input
```

### 2. JSON validation error (200 or 400)
```
Content-Type: application/json; charset=UTF-8
Body: {"code":"validation","message":"<message>"}
```

Known validation messages:
- `"at least one item is required"` (HTTP 200!)
- `"productId for item is required"` (HTTP 400)
- `"item quantity cannot be less than zero"` (HTTP 400)

### 3. JSON constraint error (422)
```
Content-Type: application/json; charset=UTF-8
Body: {"code":"constraint","message":"<message>"}
```

Known constraint messages:
- `"invalid product specified"` (HTTP 422)

### 4. Empty 404
```
Content-Length: 0
(no body)
```

### 5. Empty 405
```
Allow: <methods>
Content-Length: 0
(no body)
```

---

## Implications for Our Implementation

1. **We must implement `total` and `discounts` calculation** -- the reference API does not do this, but the spec requires it.
2. **We must implement coupon validation and discount logic** -- the reference API accepts anything, but we should validate known codes.
3. **We should enforce `api_key` authentication** -- the reference API ignores it, but the spec requires it.
4. **We should return proper error status codes** -- 400 for validation errors (not 200), proper JSON error bodies for 404s.
5. **We should validate quantity properly** -- reject negative values, reject floats, reject strings.
6. **We should decide on duplicate productId handling** -- merge/reject/accept.
7. **We should implement CORS** if browser clients need direct access.
8. **The Error schema `code` should be a string** (not int32 as spec says) to match real-world usage patterns like "validation" and "constraint". Or we could use integer codes if we want to match the spec strictly.
