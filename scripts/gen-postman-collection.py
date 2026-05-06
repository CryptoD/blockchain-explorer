#!/usr/bin/env python3
"""Emit postman/blockchain-explorer.postman_collection.json — run when API surface changes."""
import json
import sys


def url(path: str, query: str = "") -> str:
    s = "{{baseUrl}}" + path
    if query:
        s += "?" + query
    return s


CTYPE = [{"key": "Content-Type", "value": "application/json"}]
CSRF = [{"key": "X-CSRF-Token", "value": "{{csrfToken}}"}]
BEARER_API = [{"key": "Authorization", "value": "Bearer {{apiKeyBearer}}"}]


def bearer_req(
    name: str,
    method: str,
    raw_url: str,
    body=None,
    *,
    json_body: bool = False,
) -> dict:
    hdr = list(BEARER_API)
    if json_body and method in ("POST", "PUT", "PATCH"):
        hdr.extend(CTYPE)
    r = {"method": method, "header": hdr, "url": raw_url}
    if body is not None:
        r["body"] = {"mode": "raw", "raw": json.dumps(body)}
    return {"name": name, "request": r}


def get_item(name: str, raw_url: str) -> dict:
    return {"name": name, "request": {"method": "GET", "header": [], "url": raw_url}}


def req(
    name: str,
    method: str,
    raw_url: str,
    body=None,
    *,
    csrf=False,
    json_body=False,
) -> dict:
    hdr = []
    if json_body and method in ("POST", "PUT", "PATCH"):
        hdr.extend(CTYPE)
    if csrf:
        hdr.extend(CSRF)
    r = {"method": method, "header": hdr, "url": raw_url}
    if body is not None:
        r["body"] = {"mode": "raw", "raw": json.dumps(body)}
    return {"name": name, "request": r}


def merge_headers(it: dict, extra: list) -> dict:
    it = json.loads(json.dumps(it))
    it["request"]["header"] = list(extra)
    return it


def main() -> None:
    explorer = [
        get_item("Search (block/tx/address)", url("/api/v1/search", "q={{searchQuery}}")),
        get_item("Export search JSON", url("/api/v1/search/export", "q={{searchQuery}}")),
        get_item("Advanced symbol search", url("/api/v1/search/advanced", "sort_by=rank&sort_dir=asc")),
        get_item("Export advanced search JSON", url("/api/v1/search/advanced/export", "sort_by=rank")),
        get_item("Symbol categories", url("/api/v1/search/categories")),
        get_item("Blocks export CSV", url("/api/v1/blocks/export/csv")),
        get_item("Transactions export CSV", url("/api/v1/transactions/export/csv")),
        get_item("Autocomplete", url("/api/v1/autocomplete", "q=bitcoin")),
        get_item("Explorer metrics (JSON)", url("/api/v1/metrics")),
        get_item("Network status", url("/api/v1/network-status")),
        get_item("Rates", url("/api/v1/rates")),
        get_item("Price history", url("/api/v1/price-history", "currency=usd&limit=30")),
    ]

    health = [
        get_item("Liveness", url("/health")),
        get_item("Readiness", url("/ready")),
    ]

    feedback = [
        req(
            "Submit feedback",
            "POST",
            url("/api/v1/feedback"),
            {"name": "Partner", "email": "", "message": "Hello from Postman collection"},
            csrf=True,
            json_body=True,
        ),
    ]

    news = [
        get_item("News by symbol", url("/api/v1/news/{{newsSymbol}}", "page_size=10")),
        get_item("News for portfolio (auth)", url("/api/v1/news/portfolio/{{portfolioId}}", "page_size=10")),
    ]

    auth = [
        {
            "name": "Login (Tests save csrfToken)",
            "event": [
                {
                    "listen": "test",
                    "script": {
                        "type": "text/javascript",
                        "exec": [
                            "try {",
                            '  var j = pm.response.json();',
                            '  if (j.csrfToken) { pm.collectionVariables.set("csrfToken", j.csrfToken); }',
                            "} catch (e) {}",
                        ],
                    },
                }
            ],
            "request": {
                "method": "POST",
                "header": CTYPE.copy(),
                "body": {
                    "mode": "raw",
                    "raw": json.dumps({"username": "{{username}}", "password": "{{password}}"}),
                },
                "url": url("/api/v1/login"),
            },
        },
        req("Logout", "POST", url("/api/v1/logout"), csrf=True),
        req(
            "Register",
            "POST",
            url("/api/v1/register"),
            {"username": "{{registerUsername}}", "password": "{{registerPassword}}", "email": ""},
            json_body=True,
        ),
    ]

    profile = [
        get_item("Get profile", url("/api/v1/user/profile")),
        req(
            "Update profile",
            "PATCH",
            url("/api/v1/user/profile"),
            {"preferred_currency": "usd"},
            csrf=True,
            json_body=True,
        ),
        req(
            "Change password",
            "PATCH",
            url("/api/v1/user/password"),
            {"current_password": "{{password}}", "new_password": "{{newPassword}}"},
            csrf=True,
            json_body=True,
        ),
    ]

    notifications = [
        get_item("List notifications", url("/api/v1/user/notifications", "page=1&page_size=20")),
        req(
            "Update notification",
            "PUT",
            url("/api/v1/user/notifications/{{notificationId}}"),
            {"read": True},
            csrf=True,
            json_body=True,
        ),
        req("Dismiss notification", "DELETE", url("/api/v1/user/notifications/{{notificationId}}"), csrf=True),
    ]

    alerts = [
        get_item("List price alerts", url("/api/v1/user/alerts")),
        req(
            "Create price alert",
            "POST",
            url("/api/v1/user/alerts"),
            {
                "symbol": "BTC",
                "currency": "usd",
                "threshold": 100000,
                "direction": "above",
                "delivery_method": "email",
            },
            csrf=True,
            json_body=True,
        ),
        req(
            "Update price alert",
            "PUT",
            url("/api/v1/user/alerts/{{alertId}}"),
            {"is_active": True},
            csrf=True,
            json_body=True,
        ),
        req("Delete price alert", "DELETE", url("/api/v1/user/alerts/{{alertId}}"), csrf=True),
    ]

    portfolios = [
        get_item("List portfolios", url("/api/v1/user/portfolios")),
        get_item("Export portfolios JSON", url("/api/v1/user/portfolios/export")),
        req(
            "Create portfolio",
            "POST",
            url("/api/v1/user/portfolios"),
            {"name": "Demo", "description": "", "items": []},
            csrf=True,
            json_body=True,
        ),
        req(
            "Update portfolio",
            "PUT",
            url("/api/v1/user/portfolios/{{portfolioId}}"),
            {"name": "Demo", "description": "", "items": []},
            csrf=True,
            json_body=True,
        ),
        req("Delete portfolio", "DELETE", url("/api/v1/user/portfolios/{{portfolioId}}"), csrf=True),
        get_item("Export portfolio CSV", url("/api/v1/user/portfolios/{{portfolioId}}/export/csv")),
        get_item("Export portfolio PDF", url("/api/v1/user/portfolios/{{portfolioId}}/export/pdf")),
    ]

    watchlists = [
        get_item("List watchlists", url("/api/v1/user/watchlists")),
        req(
            "Create watchlist",
            "POST",
            url("/api/v1/user/watchlists"),
            {"name": "Main"},
            csrf=True,
            json_body=True,
        ),
        get_item("Get watchlist", url("/api/v1/user/watchlists/{{watchlistId}}")),
        req(
            "Update watchlist",
            "PUT",
            url("/api/v1/user/watchlists/{{watchlistId}}"),
            {"name": "Main"},
            csrf=True,
            json_body=True,
        ),
        req("Delete watchlist", "DELETE", url("/api/v1/user/watchlists/{{watchlistId}}"), csrf=True),
        req(
            "Add watchlist entry",
            "POST",
            url("/api/v1/user/watchlists/{{watchlistId}}/entries"),
            {"type": "symbol", "symbol": "bitcoin"},
            csrf=True,
            json_body=True,
        ),
        req(
            "Update watchlist entry",
            "PUT",
            url("/api/v1/user/watchlists/{{watchlistId}}/entries/{{watchlistEntryIndex}}"),
            {"type": "symbol", "symbol": "bitcoin"},
            csrf=True,
            json_body=True,
        ),
        req(
            "Delete watchlist entry",
            "DELETE",
            url("/api/v1/user/watchlists/{{watchlistId}}/entries/{{watchlistEntryIndex}}"),
            csrf=True,
        ),
    ]

    # Bearer examples (scoped API keys — no CSRF)
    user_api_keys = [
        bearer_req("List user API keys", "GET", url("/api/v1/user/api-keys")),
        bearer_req(
            "Create user API key",
            "POST",
            url("/api/v1/user/api-keys"),
            {"name": "automation", "scopes": ["user:read", "user:write"]},
            json_body=True,
        ),
        bearer_req("Revoke user API key", "DELETE", url("/api/v1/user/api-keys/{{apiKeyPublicId}}")),
    ]

    # All /api/v1/admin/* routes require CSRF (including GET) when using cookie session.
    admin = [
        merge_headers(get_item("Admin status", url("/api/v1/admin/status")), CSRF),
        merge_headers(get_item("Admin cache stats", url("/api/v1/admin/cache", "action=stats")), CSRF),
        merge_headers(get_item("Admin status (Bearer service key)", url("/api/v1/admin/status")), BEARER_API),
        merge_headers(
            get_item("Admin cache stats (Bearer service key)", url("/api/v1/admin/cache", "action=stats")),
            BEARER_API,
        ),
        merge_headers(get_item("List service API keys (Bearer)", url("/api/v1/admin/api-keys")), BEARER_API),
        req(
            "Create service API key (session + CSRF only)",
            "POST",
            url("/api/v1/admin/api-keys"),
            {"name": "partner", "label": "", "scopes": ["admin:read"]},
            csrf=True,
            json_body=True,
        ),
    ]

    collection = {
        "info": {
            "_postman_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
            "name": "Bitcoin Explorer — /api/v1",
            "description": (
                "Versioned REST API (`/api/v1`). Source of truth: repository `openapi.yaml`. "
                "Import this file into Postman or Bruno.\n\n"
                "**Auth:** Cookie sessions. Run **Login**; Postman persists `Set-Cookie` for `session_id`. "
                "The login **Tests** script stores `csrfToken` for mutating routes.\n\n"
                "**Machine tokens:** optional `Authorization: Bearer {{apiKeyBearer}}` (prefix `bkx_`) "
                "for scoped automation (`user:*` / `admin:*`). No CSRF when you do **not** send `session_id`. "
                "See docs/API_KEYS.md.\n\n"
                "**CSRF:** Send `X-CSRF-Token` on POST/PUT/PATCH/DELETE and on **all** `/api/v1/admin/*` requests "
                "when logged in (`csrfMiddleware`). Register/login skip CSRF by design.\n\n"
                "**Probe:** Prometheus scrape is `GET /metrics` with optional `METRICS_TOKEN` — omitted here."
            ),
            "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json",
        },
        "variable": [
            {"key": "baseUrl", "value": "http://localhost:8080"},
            {"key": "csrfToken", "value": ""},
            {"key": "searchQuery", "value": "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"},
            {"key": "newsSymbol", "value": "BTC"},
            {"key": "username", "value": ""},
            {"key": "password", "value": ""},
            {"key": "registerUsername", "value": ""},
            {"key": "registerPassword", "value": ""},
            {"key": "newPassword", "value": ""},
            {"key": "portfolioId", "value": ""},
            {"key": "alertId", "value": ""},
            {"key": "notificationId", "value": ""},
            {"key": "watchlistId", "value": ""},
            {"key": "watchlistEntryIndex", "value": "0"},
            {"key": "apiKeyBearer", "value": ""},
            {"key": "apiKeyPublicId", "value": ""},
        ],
        "item": [
            {"name": "Health", "item": health},
            {"name": "Explorer (public)", "item": explorer},
            {"name": "Feedback", "item": feedback},
            {"name": "News", "item": news},
            {"name": "Auth", "item": auth},
            {"name": "User — profile", "item": profile},
            {"name": "User — notifications", "item": notifications},
            {"name": "User — price alerts", "item": alerts},
            {"name": "User — portfolios", "item": portfolios},
            {"name": "User — watchlists", "item": watchlists},
            {"name": "User — API keys (Bearer)", "item": user_api_keys},
            {"name": "Admin (admin role)", "item": admin},
        ],
    }

    sys.stdout.write(json.dumps(collection, indent=2, ensure_ascii=True))
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
