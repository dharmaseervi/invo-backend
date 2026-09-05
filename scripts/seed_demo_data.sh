#!/usr/bin/env bash
# Seeds a demo account on production with realistic-looking data for App Store screenshots.
# You only need to do the signup + email OTP step yourself (see STEP 1) — everything
# else (company, clients, items, invoices with varied statuses, an estimate) is scripted.
#
# Usage:
#   1. Edit EMAIL/PASSWORD below if you want different ones.
#   2. Run: bash scripts/seed_demo_data.sh
#   3. It will pause and ask you to paste the OTP code emailed to EMAIL.
#   4. It then creates everything else automatically.

set -euo pipefail

BASE="https://invobilling.com/api/v1"
EMAIL="dharmaseervijb18239+demo@gmail.com"
PASSWORD="DemoScreens#2026"

echo "=== Invo Billing demo data seeder ==="
echo "Registering $EMAIL ..."

REGISTER_RESP=$(curl -s -X POST "$BASE/register" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")
echo "$REGISTER_RESP"

if echo "$REGISTER_RESP" | grep -q "already registered"; then
  echo "Account already exists — skipping to login. If it's not verified yet, verification will fail below."
fi

echo ""
echo "Check $EMAIL's inbox for the OTP code, then paste it here:"
read -r OTP_CODE

curl -s -X POST "$BASE/verify-email" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL\",\"code\":\"$OTP_CODE\"}"
echo ""

echo "Logging in ..."
LOGIN_RESP=$(curl -s -X POST "$BASE/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")

TOKEN=$(echo "$LOGIN_RESP" | grep -o '"token":"[^"]*"' | head -1 | cut -d'"' -f4)
if [ -z "$TOKEN" ]; then
  TOKEN=$(echo "$LOGIN_RESP" | grep -o '"access_token":"[^"]*"' | head -1 | cut -d'"' -f4)
fi
if [ -z "$TOKEN" ]; then
  echo "Login failed, response was:"
  echo "$LOGIN_RESP"
  exit 1
fi
echo "Logged in."

auth() { curl -s -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" "$@"; }

echo ""
echo "Creating company ..."
COMPANY_RESP=$(auth -X POST "$BASE/companies" -d '{
  "name": "Sharma Textiles & Traders",
  "address": "14 Ring Road, Sector 21",
  "phone": "9876543210",
  "gst": "07ABCDE1234F1Z5",
  "city": "New Delhi",
  "state": "Delhi",
  "pincode": "110021"
}')
echo "$COMPANY_RESP"
COMPANY_ID=$(echo "$COMPANY_RESP" | grep -o '"company_id":[0-9]*' | grep -o '[0-9]*')
echo "Company ID: $COMPANY_ID"

echo ""
echo "Creating category ..."
auth -X POST "$BASE/categories" -d "{\"name\":\"General\",\"company_id\":$COMPANY_ID}"
echo ""
CATEGORY_ID=$(auth "$BASE/categories/$COMPANY_ID" | grep -o '"id":[0-9]*' | head -1 | grep -o '[0-9]*')
echo "Category ID: $CATEGORY_ID"

echo ""
echo "Creating clients ..."
create_client() {
  auth -X POST "$BASE/clients" -d "$1"
}
create_client "{\"name\":\"Aarav Enterprises\",\"email\":\"accounts@aaravent.in\",\"phone\":\"9812345678\",\"address\":\"22 MG Road\",\"city\":\"Bengaluru\",\"state\":\"Karnataka\",\"pincode\":\"560001\",\"company_id\":$COMPANY_ID}"
echo ""
create_client "{\"name\":\"Priya Retail Store\",\"email\":\"priya.store@gmail.com\",\"phone\":\"9823456789\",\"address\":\"5 Linking Road\",\"city\":\"Mumbai\",\"state\":\"Maharashtra\",\"pincode\":\"400050\",\"company_id\":$COMPANY_ID}"
echo ""
create_client "{\"name\":\"Kiran Fabrics Pvt Ltd\",\"email\":\"purchase@kiranfabrics.com\",\"phone\":\"9834567890\",\"address\":\"88 Anna Salai\",\"city\":\"Chennai\",\"state\":\"Tamil Nadu\",\"pincode\":\"600002\",\"company_id\":$COMPANY_ID}"
echo ""

CLIENT_IDS=$(auth "$BASE/companies/$COMPANY_ID/clients" | grep -o '"id":[0-9]*' | grep -o '[0-9]*')
CLIENT_1=$(echo "$CLIENT_IDS" | sed -n '1p')
CLIENT_2=$(echo "$CLIENT_IDS" | sed -n '2p')
CLIENT_3=$(echo "$CLIENT_IDS" | sed -n '3p')
echo "Client IDs: $CLIENT_1 $CLIENT_2 $CLIENT_3"

echo ""
echo "Creating items ..."
create_item() {
  auth -X POST "$BASE/items" -d "$1"
}
create_item "{\"name\":\"Cotton Fabric Roll (per meter)\",\"category_id\":$CATEGORY_ID,\"sku\":\"CFR-001\",\"unit\":\"meter\",\"description\":\"Premium cotton fabric\",\"cost_price\":120,\"price\":180,\"quantity\":500,\"low_stock_alert\":50,\"tax_rate\":5,\"hsn_code\":\"5208\",\"company_id\":$COMPANY_ID}"
echo ""
create_item "{\"name\":\"Silk Saree - Banarasi\",\"category_id\":$CATEGORY_ID,\"sku\":\"SSB-014\",\"unit\":\"piece\",\"description\":\"Handwoven Banarasi silk saree\",\"cost_price\":2200,\"price\":3500,\"quantity\":40,\"low_stock_alert\":5,\"tax_rate\":12,\"hsn_code\":\"5007\",\"company_id\":$COMPANY_ID}"
echo ""
create_item "{\"name\":\"Cotton Kurta (Men)\",\"category_id\":$CATEGORY_ID,\"sku\":\"CKM-027\",\"unit\":\"piece\",\"description\":\"Casual cotton kurta\",\"cost_price\":350,\"price\":550,\"quantity\":150,\"low_stock_alert\":20,\"tax_rate\":5,\"hsn_code\":\"6205\",\"company_id\":$COMPANY_ID}"
echo ""

ITEM_IDS=$(auth "$BASE/items/$COMPANY_ID/all" | grep -o '"id":[0-9]*' | grep -o '[0-9]*')
ITEM_1=$(echo "$ITEM_IDS" | sed -n '1p')
ITEM_2=$(echo "$ITEM_IDS" | sed -n '2p')
ITEM_3=$(echo "$ITEM_IDS" | sed -n '3p')
echo "Item IDs: $ITEM_1 $ITEM_2 $ITEM_3"

echo ""
echo "Creating invoices (draft) ..."
create_invoice() {
  auth -X POST "$BASE/invoices" -d "$1"
}

# Invoice 1: will be issued + fully paid
INV1_RESP=$(create_invoice "{\"company_id\":$COMPANY_ID,\"client_id\":$CLIENT_1,\"invoice_date\":\"2026-08-20\",\"due_date\":\"2026-09-04\",\"discount\":0,\"items\":[{\"item_id\":$ITEM_1,\"qty\":20,\"rate\":180,\"discount\":0,\"tax_rate\":5},{\"item_id\":$ITEM_3,\"qty\":10,\"rate\":550,\"discount\":0,\"tax_rate\":5}]}")
echo "$INV1_RESP"
INV1_ID=$(echo "$INV1_RESP" | grep -o '"id":[0-9]*' | head -1 | grep -o '[0-9]*')

# Invoice 2: will be issued + partially paid
INV2_RESP=$(create_invoice "{\"company_id\":$COMPANY_ID,\"client_id\":$CLIENT_2,\"invoice_date\":\"2026-08-28\",\"due_date\":\"2026-09-12\",\"discount\":200,\"items\":[{\"item_id\":$ITEM_2,\"qty\":3,\"rate\":3500,\"discount\":0,\"tax_rate\":12}]}")
echo "$INV2_RESP"
INV2_ID=$(echo "$INV2_RESP" | grep -o '"id":[0-9]*' | head -1 | grep -o '[0-9]*')

# Invoice 3: will be issued, left unpaid + overdue (due date in the past)
INV3_RESP=$(create_invoice "{\"company_id\":$COMPANY_ID,\"client_id\":$CLIENT_3,\"invoice_date\":\"2026-08-01\",\"due_date\":\"2026-08-15\",\"discount\":0,\"items\":[{\"item_id\":$ITEM_1,\"qty\":50,\"rate\":180,\"discount\":0,\"tax_rate\":5}]}")
echo "$INV3_RESP"
INV3_ID=$(echo "$INV3_RESP" | grep -o '"id":[0-9]*' | head -1 | grep -o '[0-9]*')

# Invoice 4: left as draft (shows the draft state in the list)
create_invoice "{\"company_id\":$COMPANY_ID,\"client_id\":$CLIENT_1,\"invoice_date\":\"2026-09-05\",\"due_date\":\"2026-09-20\",\"discount\":0,\"items\":[{\"item_id\":$ITEM_3,\"qty\":5,\"rate\":550,\"discount\":0,\"tax_rate\":5}]}"
echo ""

echo ""
echo "Issuing invoices 1-3 ..."
auth -X POST "$BASE/invoices/$INV1_ID/issue"
echo ""
auth -X POST "$BASE/invoices/$INV2_ID/issue"
echo ""
auth -X POST "$BASE/invoices/$INV3_ID/issue"
echo ""

echo ""
echo "Recording payments ..."
# Full payment for invoice 1 (20*180 + 10*550 = 9100, +5% tax = 9555)
auth -X POST "$BASE/payments" -d "{\"client_id\":$CLIENT_1,\"amount\":9555,\"payment_method\":\"bank_transfer\",\"reference\":\"UTR203948\",\"notes\":\"Full payment\"}"
echo ""
# Partial payment for invoice 2
auth -X POST "$BASE/payments" -d "{\"client_id\":$CLIENT_2,\"amount\":5000,\"payment_method\":\"upi\",\"reference\":\"UPI778812\",\"notes\":\"Partial payment\"}"
echo ""

echo ""
echo "Creating an estimate ..."
auth -X POST "$BASE/estimates" -d "{\"company_id\":$COMPANY_ID,\"client_id\":$CLIENT_3,\"estimate_date\":\"2026-09-05\",\"expiry_date\":\"2026-09-25\",\"discount\":0,\"items\":[{\"item_id\":$ITEM_2,\"qty\":2,\"rate\":3500,\"discount\":0,\"tax_rate\":12}]}"
echo ""

echo ""
echo "=== Done ==="
echo "Log into the app with:"
echo "  Email:    $EMAIL"
echo "  Password: $PASSWORD"
echo "You should see: 1 company, 3 clients, 3 items, 4 invoices (paid / partial / overdue / draft), 1 estimate."
