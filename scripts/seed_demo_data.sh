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

# Dates are generated relative to the day the script runs. They used to be hard-coded
# to August/September 2026, which meant that a month later the dashboard's "Week" view
# — the first thing anyone sees, reviewer included — showed a total revenue of ₹0.00
# and "No sales recorded in the last 7 days". Demo data has to stay recent to keep
# looking like a working business.
d() { # d <days-ago> -> YYYY-MM-DD
  if date -v -1d +%Y-%m-%d >/dev/null 2>&1; then date -v -"$1"d +%Y-%m-%d   # BSD/macOS
  else date -d "$1 days ago" +%Y-%m-%d; fi                                  # GNU
}
dplus() { # dplus <days-ahead> -> YYYY-MM-DD
  if date -v +1d +%Y-%m-%d >/dev/null 2>&1; then date -v +"$1"d +%Y-%m-%d
  else date -d "$1 days" +%Y-%m-%d; fi
}

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
echo "Creating categories ..."
# Four categories rather than one: the stock report groups by category, and a single
# "General" bucket makes that screen look like a list with a redundant heading.
for c in Fabric Sarees Readymade Accessories; do
  auth -X POST "$BASE/categories" -d "{\"name\":\"$c\",\"company_id\":$COMPANY_ID}" >/dev/null
done
CAT_IDS=$(auth "$BASE/categories/$COMPANY_ID" | grep -o '"id":[0-9]*' | grep -o '[0-9]*')
CAT_FABRIC=$(echo "$CAT_IDS" | sed -n '1p')
CAT_SAREE=$(echo "$CAT_IDS" | sed -n '2p')
CAT_READY=$(echo "$CAT_IDS" | sed -n '3p')
CAT_ACC=$(echo "$CAT_IDS" | sed -n '4p')
echo "Categories: $CAT_FABRIC $CAT_SAREE $CAT_READY $CAT_ACC"

echo ""
echo "Creating clients ..."
# The company is registered in Delhi, so a Delhi client is billed CGST+SGST and an
# out-of-state client is billed IGST. Both kinds are seeded deliberately — with only
# out-of-state clients the GST report shows an empty CGST/SGST column.
new_client() { # new_client <json> -> echoes the new client id
  auth -X POST "$BASE/clients" -d "$1" | grep -o '"client_id":[0-9]*' | grep -o '[0-9]*'
}
C1=$(new_client "{\"name\":\"Aarav Enterprises\",\"email\":\"accounts@aaravent.in\",\"phone\":\"9812345678\",\"address\":\"22 MG Road\",\"city\":\"Bengaluru\",\"state\":\"Karnataka\",\"pincode\":\"560001\",\"company_id\":$COMPANY_ID}")
C2=$(new_client "{\"name\":\"Priya Retail Store\",\"email\":\"priya.store@example.in\",\"phone\":\"9823456789\",\"address\":\"5 Linking Road\",\"city\":\"Mumbai\",\"state\":\"Maharashtra\",\"pincode\":\"400050\",\"company_id\":$COMPANY_ID}")
C3=$(new_client "{\"name\":\"Kiran Fabrics Pvt Ltd\",\"email\":\"purchase@kiranfabrics.example\",\"phone\":\"9834567890\",\"address\":\"88 Anna Salai\",\"city\":\"Chennai\",\"state\":\"Tamil Nadu\",\"pincode\":\"600002\",\"company_id\":$COMPANY_ID}")
C4=$(new_client "{\"name\":\"Naveen Garments\",\"email\":\"naveen@navgarments.example\",\"phone\":\"9845678901\",\"address\":\"31 Karol Bagh\",\"city\":\"New Delhi\",\"state\":\"Delhi\",\"pincode\":\"110005\",\"company_id\":$COMPANY_ID}")
C5=$(new_client "{\"name\":\"Meera Boutique\",\"email\":\"hello@meeraboutique.example\",\"phone\":\"9856789012\",\"address\":\"7 Hauz Khas Village\",\"city\":\"New Delhi\",\"state\":\"Delhi\",\"pincode\":\"110016\",\"company_id\":$COMPANY_ID}")
C6=$(new_client "{\"name\":\"Rajesh Cloth House\",\"email\":\"rajesh@clothhouse.example\",\"phone\":\"9867890123\",\"address\":\"12 Johari Bazaar\",\"city\":\"Jaipur\",\"state\":\"Rajasthan\",\"pincode\":\"302003\",\"company_id\":$COMPANY_ID}")
C7=$(new_client "{\"name\":\"Sunrise Traders\",\"email\":\"orders@sunrisetraders.example\",\"phone\":\"9878901234\",\"address\":\"44 CG Road\",\"city\":\"Ahmedabad\",\"state\":\"Gujarat\",\"pincode\":\"380009\",\"company_id\":$COMPANY_ID}")
C8=$(new_client "{\"name\":\"Lakshmi Silks\",\"email\":\"billing@lakshmisilks.example\",\"phone\":\"9889012345\",\"address\":\"9 Charminar Road\",\"city\":\"Hyderabad\",\"state\":\"Telangana\",\"pincode\":\"500002\",\"company_id\":$COMPANY_ID}")
echo "Clients: $C1 $C2 $C3 $C4 $C5 $C6 $C7 $C8"

echo ""
echo "Creating items ..."
# Tax rates deliberately span 5%, 12% and 18% so the GST report's HSN summary has
# more than one row to show.
create_item() { auth -X POST "$BASE/items" -d "$1" >/dev/null; }
create_item "{\"name\":\"Cotton Fabric Roll\",\"category_id\":$CAT_FABRIC,\"sku\":\"CFR-001\",\"unit\":\"meter\",\"description\":\"Premium combed cotton, 44 inch\",\"cost_price\":120,\"price\":180,\"quantity\":480,\"low_stock_alert\":50,\"tax_rate\":5,\"hsn_code\":\"5208\",\"company_id\":$COMPANY_ID}"
create_item "{\"name\":\"Linen Fabric Roll\",\"category_id\":$CAT_FABRIC,\"sku\":\"LFR-002\",\"unit\":\"meter\",\"description\":\"Pure linen, natural finish\",\"cost_price\":260,\"price\":395,\"quantity\":210,\"low_stock_alert\":40,\"tax_rate\":5,\"hsn_code\":\"5309\",\"company_id\":$COMPANY_ID}"
create_item "{\"name\":\"Denim Fabric Roll\",\"category_id\":$CAT_FABRIC,\"sku\":\"DFR-003\",\"unit\":\"meter\",\"description\":\"12 oz indigo denim\",\"cost_price\":180,\"price\":265,\"quantity\":36,\"low_stock_alert\":40,\"tax_rate\":5,\"hsn_code\":\"5209\",\"company_id\":$COMPANY_ID}"
create_item "{\"name\":\"Banarasi Silk Saree\",\"category_id\":$CAT_SAREE,\"sku\":\"SSB-014\",\"unit\":\"piece\",\"description\":\"Handwoven Banarasi silk, zari border\",\"cost_price\":2200,\"price\":3500,\"quantity\":42,\"low_stock_alert\":6,\"tax_rate\":12,\"hsn_code\":\"5007\",\"company_id\":$COMPANY_ID}"
create_item "{\"name\":\"Kanjivaram Silk Saree\",\"category_id\":$CAT_SAREE,\"sku\":\"SSK-015\",\"unit\":\"piece\",\"description\":\"Pure Kanjivaram, contrast pallu\",\"cost_price\":4100,\"price\":6250,\"quantity\":18,\"low_stock_alert\":5,\"tax_rate\":12,\"hsn_code\":\"5007\",\"company_id\":$COMPANY_ID}"
create_item "{\"name\":\"Cotton Handloom Saree\",\"category_id\":$CAT_SAREE,\"sku\":\"SCH-016\",\"unit\":\"piece\",\"description\":\"Handloom cotton, block print\",\"cost_price\":650,\"price\":1050,\"quantity\":64,\"low_stock_alert\":10,\"tax_rate\":5,\"hsn_code\":\"5208\",\"company_id\":$COMPANY_ID}"
create_item "{\"name\":\"Cotton Kurta (Men)\",\"category_id\":$CAT_READY,\"sku\":\"CKM-027\",\"unit\":\"piece\",\"description\":\"Casual cotton kurta, full sleeve\",\"cost_price\":350,\"price\":550,\"quantity\":150,\"low_stock_alert\":20,\"tax_rate\":5,\"hsn_code\":\"6205\",\"company_id\":$COMPANY_ID}"
create_item "{\"name\":\"Ladies Salwar Suit\",\"category_id\":$CAT_READY,\"sku\":\"LSS-028\",\"unit\":\"set\",\"description\":\"Unstitched 3-piece suit\",\"cost_price\":720,\"price\":1150,\"quantity\":88,\"low_stock_alert\":15,\"tax_rate\":5,\"hsn_code\":\"6204\",\"company_id\":$COMPANY_ID}"
create_item "{\"name\":\"Kids T-Shirt\",\"category_id\":$CAT_READY,\"sku\":\"KTS-029\",\"unit\":\"piece\",\"description\":\"Cotton round neck, assorted\",\"cost_price\":140,\"price\":245,\"quantity\":0,\"low_stock_alert\":25,\"tax_rate\":5,\"hsn_code\":\"6109\",\"company_id\":$COMPANY_ID}"
create_item "{\"name\":\"Formal Shirt (Men)\",\"category_id\":$CAT_READY,\"sku\":\"FSM-030\",\"unit\":\"piece\",\"description\":\"Wrinkle-free cotton blend\",\"cost_price\":540,\"price\":899,\"quantity\":74,\"low_stock_alert\":12,\"tax_rate\":12,\"hsn_code\":\"6205\",\"company_id\":$COMPANY_ID}"
create_item "{\"name\":\"Silk Dupatta\",\"category_id\":$CAT_ACC,\"sku\":\"SDP-041\",\"unit\":\"piece\",\"description\":\"Art silk, tassel ends\",\"cost_price\":190,\"price\":340,\"quantity\":120,\"low_stock_alert\":20,\"tax_rate\":5,\"hsn_code\":\"6214\",\"company_id\":$COMPANY_ID}"
create_item "{\"name\":\"Leather Belt\",\"category_id\":$CAT_ACC,\"sku\":\"LBT-042\",\"unit\":\"piece\",\"description\":\"Genuine leather, steel buckle\",\"cost_price\":310,\"price\":599,\"quantity\":52,\"low_stock_alert\":10,\"tax_rate\":18,\"hsn_code\":\"4203\",\"company_id\":$COMPANY_ID}"

ITEM_IDS=$(auth "$BASE/items/$COMPANY_ID/all" | grep -o '"id":[0-9]*' | grep -o '[0-9]*')
I1=$(echo "$ITEM_IDS" | sed -n '1p');  I2=$(echo "$ITEM_IDS" | sed -n '2p')
I3=$(echo "$ITEM_IDS" | sed -n '3p');  I4=$(echo "$ITEM_IDS" | sed -n '4p')
I5=$(echo "$ITEM_IDS" | sed -n '5p');  I6=$(echo "$ITEM_IDS" | sed -n '6p')
I7=$(echo "$ITEM_IDS" | sed -n '7p');  I8=$(echo "$ITEM_IDS" | sed -n '8p')
I10=$(echo "$ITEM_IDS" | sed -n '10p'); I11=$(echo "$ITEM_IDS" | sed -n '11p')
I12=$(echo "$ITEM_IDS" | sed -n '12p')
echo "Items: $I1 .. $I12"

echo ""
echo "Creating invoices ..."
new_invoice() { auth -X POST "$BASE/invoices" -d "$1" | grep -o '"id":[0-9]*' | head -1 | grep -o '[0-9]*'; }

# Recent and paid — keeps the dashboard's 7-day revenue figure non-zero.
INV1=$(new_invoice "{\"company_id\":$COMPANY_ID,\"client_id\":$C1,\"invoice_date\":\"$(d 2)\",\"due_date\":\"$(dplus 13)\",\"discount\":0,\"items\":[{\"item_id\":$I1,\"qty\":60,\"rate\":180,\"discount\":0,\"tax_rate\":5},{\"item_id\":$I7,\"qty\":25,\"rate\":550,\"discount\":0,\"tax_rate\":5}]}")
INV2=$(new_invoice "{\"company_id\":$COMPANY_ID,\"client_id\":$C4,\"invoice_date\":\"$(d 3)\",\"due_date\":\"$(dplus 12)\",\"discount\":0,\"items\":[{\"item_id\":$I4,\"qty\":6,\"rate\":3500,\"discount\":0,\"tax_rate\":12}]}")
INV3=$(new_invoice "{\"company_id\":$COMPANY_ID,\"client_id\":$C5,\"invoice_date\":\"$(d 5)\",\"due_date\":\"$(dplus 10)\",\"discount\":250,\"items\":[{\"item_id\":$I8,\"qty\":18,\"rate\":1150,\"discount\":0,\"tax_rate\":5},{\"item_id\":$I11,\"qty\":30,\"rate\":340,\"discount\":0,\"tax_rate\":5}]}")
# Partly paid
INV4=$(new_invoice "{\"company_id\":$COMPANY_ID,\"client_id\":$C2,\"invoice_date\":\"$(d 9)\",\"due_date\":\"$(dplus 6)\",\"discount\":0,\"items\":[{\"item_id\":$I5,\"qty\":4,\"rate\":6250,\"discount\":0,\"tax_rate\":12}]}")
INV5=$(new_invoice "{\"company_id\":$COMPANY_ID,\"client_id\":$C7,\"invoice_date\":\"$(d 14)\",\"due_date\":\"$(dplus 1)\",\"discount\":0,\"items\":[{\"item_id\":$I10,\"qty\":22,\"rate\":899,\"discount\":0,\"tax_rate\":12},{\"item_id\":$I12,\"qty\":15,\"rate\":599,\"discount\":0,\"tax_rate\":18}]}")
# Overdue
INV6=$(new_invoice "{\"company_id\":$COMPANY_ID,\"client_id\":$C3,\"invoice_date\":\"$(d 40)\",\"due_date\":\"$(d 12)\",\"discount\":0,\"items\":[{\"item_id\":$I2,\"qty\":45,\"rate\":395,\"discount\":0,\"tax_rate\":5}]}")
INV7=$(new_invoice "{\"company_id\":$COMPANY_ID,\"client_id\":$C6,\"invoice_date\":\"$(d 55)\",\"due_date\":\"$(d 25)\",\"discount\":0,\"items\":[{\"item_id\":$I6,\"qty\":20,\"rate\":1050,\"discount\":0,\"tax_rate\":5}]}")
INV8=$(new_invoice "{\"company_id\":$COMPANY_ID,\"client_id\":$C8,\"invoice_date\":\"$(d 70)\",\"due_date\":\"$(d 40)\",\"discount\":0,\"items\":[{\"item_id\":$I3,\"qty\":28,\"rate\":265,\"discount\":0,\"tax_rate\":5}]}")
# Left as drafts
new_invoice "{\"company_id\":$COMPANY_ID,\"client_id\":$C1,\"invoice_date\":\"$(d 0)\",\"due_date\":\"$(dplus 15)\",\"discount\":0,\"items\":[{\"item_id\":$I7,\"qty\":10,\"rate\":550,\"discount\":0,\"tax_rate\":5}]}" >/dev/null
new_invoice "{\"company_id\":$COMPANY_ID,\"client_id\":$C5,\"invoice_date\":\"$(d 1)\",\"due_date\":\"$(dplus 14)\",\"discount\":0,\"items\":[{\"item_id\":$I11,\"qty\":12,\"rate\":340,\"discount\":0,\"tax_rate\":5}]}" >/dev/null
echo "Invoices: $INV1 $INV2 $INV3 $INV4 $INV5 $INV6 $INV7 $INV8 (+2 drafts)"

echo ""
echo "Issuing invoices ..."
for id in $INV1 $INV2 $INV3 $INV4 $INV5 $INV6 $INV7 $INV8; do
  auth -X POST "$BASE/invoices/$id/issue" >/dev/null
done
echo "done"

echo ""
echo "Recording payments ..."
pay() { auth -X POST "$BASE/payments" -d "$1" >/dev/null; }
pay "{\"client_id\":$C1,\"amount\":25777.50,\"payment_method\":\"bank_transfer\",\"reference\":\"UTR203948\",\"notes\":\"Full payment\"}"
pay "{\"client_id\":$C4,\"amount\":23520,\"payment_method\":\"upi\",\"reference\":\"UPI778812\",\"notes\":\"Full payment\"}"
pay "{\"client_id\":$C5,\"amount\":15000,\"payment_method\":\"cash\",\"reference\":\"CASH-118\",\"notes\":\"Part payment\"}"
pay "{\"client_id\":$C2,\"amount\":10000,\"payment_method\":\"upi\",\"reference\":\"UPI889931\",\"notes\":\"Advance\"}"
echo "done"

echo ""
echo "Recording expenses ..."
spend() { auth -X POST "$BASE/expenses" -d "$1" >/dev/null; }
spend "{\"company_id\":$COMPANY_ID,\"name\":\"Shop rent\",\"amount\":28000,\"description\":\"Monthly rent — Sector 21\",\"date\":\"$(d 6)\"}"
spend "{\"company_id\":$COMPANY_ID,\"name\":\"Electricity bill\",\"amount\":4380,\"description\":\"BSES, August\",\"date\":\"$(d 9)\"}"
spend "{\"company_id\":$COMPANY_ID,\"name\":\"Transport\",\"amount\":6200,\"description\":\"Delivery to Bengaluru\",\"date\":\"$(d 12)\"}"
spend "{\"company_id\":$COMPANY_ID,\"name\":\"Packaging material\",\"amount\":3150,\"description\":\"Covers and tape\",\"date\":\"$(d 18)\"}"
spend "{\"company_id\":$COMPANY_ID,\"name\":\"Staff salary\",\"amount\":42000,\"description\":\"2 staff, monthly\",\"date\":\"$(d 22)\"}"
echo "done"

echo ""
echo "Creating estimates ..."
auth -X POST "$BASE/estimates" -d "{\"company_id\":$COMPANY_ID,\"client_id\":$C3,\"estimate_date\":\"$(d 2)\",\"expiry_date\":\"$(dplus 18)\",\"discount\":0,\"items\":[{\"item_id\":$I4,\"qty\":8,\"rate\":3500,\"discount\":0,\"tax_rate\":12}]}" >/dev/null
auth -X POST "$BASE/estimates" -d "{\"company_id\":$COMPANY_ID,\"client_id\":$C6,\"estimate_date\":\"$(d 7)\",\"expiry_date\":\"$(dplus 8)\",\"discount\":0,\"items\":[{\"item_id\":$I8,\"qty\":25,\"rate\":1150,\"discount\":0,\"tax_rate\":5},{\"item_id\":$I11,\"qty\":40,\"rate\":340,\"discount\":0,\"tax_rate\":5}]}" >/dev/null
echo "done"

echo ""
echo "=== Done ==="
echo "Log into the app with:"
echo "  Email:    $EMAIL"
echo "  Password: $PASSWORD"
echo "You should see: 1 company, 8 clients, 12 items across 4 categories,"
echo "10 invoices (paid / part-paid / overdue / draft), 4 payments, 5 expenses,"
echo "2 estimates. One item is out of stock and one is below its low-stock level,"
echo "so the stock report has something to flag."
