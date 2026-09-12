# App Store review notes

Paste section 1 into **App Store Connect → your build → App Review Information →
Notes for Review**. It is written to satisfy Guideline 2.3.1, which requires features
to be "described with specificity" and warns that "generic descriptions will be
rejected".

Sections 2 and 3 are for you, not for Apple.

---

## 1. Notes for Review — paste this

> **What this app is**
>
> Invo Billing is GST billing and inventory software for small retailers and traders in
> India. It creates invoices that comply with Indian GST rules, tracks stock, and
> produces the summaries needed to file a GSTR-1 return. It is bookkeeping software in
> the same category as Zoho Invoice, Vyapar or QuickBooks.
>
> **It does not handle, hold, transfer or process money.** There is no payment gateway,
> no card entry, no wallet, no bank connection and no in-app purchase. Where the app
> refers to a "payment", it is the user typing in a record of cash or a bank transfer
> that already happened outside the app, exactly as they would write it in a paper
> ledger.
>
> **Demo account**
>
> Email: appreview@invobilling.com
> Password: [FILL IN]
>
> The account is email-verified and pre-loaded with a sample business — "Sharma
> Hardware & Paints" — containing 4 clients, 8 stock items, 6 invoices, 1 estimate,
> 3 recorded payments and 4 expenses, so every screen has real data on first launch.
> No further sign-up, OTP or verification step is needed.
>
> **Every feature, and how to reach it**
>
> The app has five tabs: Home, Invoices, Clients, Items, More.
>
> 1. **Home** — dashboard. Revenue for the selected period, counts of invoices, clients
>    and items, a seven-day revenue chart, and the most recent invoices.
> 2. **Invoices** — list of invoices. Tap "+" to create one: choose a client, search
>    and add items, set quantity, rate, per-line discount and GST rate. Save as a draft,
>    then "Issue" to assign the invoice number and deduct stock. An issued invoice can
>    be cancelled, exported as a PDF, or emailed to the client.
> 3. **Clients** — customer records: name, phone, email, address, state and GSTIN. Tap
>    a client for their invoices, outstanding balance and account ledger.
> 4. **Items** — product catalogue with SKU, HSN code, unit, cost price, selling price,
>    GST rate, stock quantity and a low-stock threshold. Tap an item to record stock
>    received and to see its movement history. Items can be found by scanning a barcode
>    using the device camera (permission requested on first use).
>    **Label printing:** items can be printed as shelf labels on a Brother QL-series
>    label printer over Wi-Fi or Bluetooth. This needs the Local Network permission and
>    a physical Brother printer. A reviewer without that hardware will see the printer
>    search screen find no devices; this is expected and is not a hidden feature.
> 5. **More** — everything else, all reachable from this one screen:
>    - **Estimates** — quotations. Can be marked sent, accepted or rejected, and
>      converted into an invoice.
>    - **Credit notes** — for goods returned or a price corrected after invoicing.
>      Returns stock and reduces the GST owed on that supply.
>    - **Payments** — record a payment received; it is applied to that client's oldest
>      unpaid invoices.
>    - **Expenses** — business costs such as rent or electricity.
>    - **Ledger** — every invoice, payment and credit note per client, with a running
>      balance.
>    - **Reports** — Stock on hand, Receivables ageing, and GSTR-1 (the GST return
>      summary, exportable as CSV).
>    - **Company settings** — business name, GSTIN, address and bank details, which are
>      printed on the invoice PDF.
>    - **Invoice template** — choose between three PDF layouts.
>    - **Biometric lock** — optional Face ID / Touch ID lock on app open.
>    - **Account** — sign out, or delete the account and all its data.
>
> **Permissions, and why**
>
> - *Camera* — scanning a product barcode to find an item. Optional; items can always
>   be found by typing.
> - *Photo Library* — reading a barcode from a photo the user already has, as an
>   alternative to pointing the camera at it. Optional.
> - *Local Network* — discovering a Brother label printer on the same Wi-Fi. Optional;
>   only used on the label printing screen.
> - *Face ID* — the optional app lock described above. Off by default.
> - *Notifications* — reminders about invoices that have become overdue. Optional.
>
> **There are no hidden, dormant or undocumented features.** The app contains no web
> views and loads no remote code or remote configuration. Development-only helpers are
> excluded from release builds at compile time and are not present in this binary.
>
> **A web version** of the same account is available at https://invobilling.com/app —
> the same demo credentials work there if that is easier to review.

---

## 2. Before you resubmit — check each of these

- [ ] **The demo password above is filled in, and you have signed in with it yourself**,
      on a device, today. A demo account that does not work is the single commonest
      cause of a 2.3.1 rejection.
- [ ] **Primary category is Business, not Finance.** Zoho Invoice, Vyapar, myBillBook
      and QuickBooks are all listed under Business. Finance invites review against the
      financial-services rules, which is how 3.2.1(viii) came up.
- [ ] **Support URL and Privacy Policy URL** both resolve and describe this app.
- [ ] **Screenshots** show the app with the demo data in it, not empty screens.
- [ ] **Age rating** questionnaire completed.
- [ ] **Account deletion** is reachable in-app (it is, under More → Account) — required
      by Guideline 5.1.1(v).

---

## 3. If 3.2.1(viii) is raised again

The guideline reads:

> "Apps used for financial trading, investing, or money management should be submitted
> by the financial institution performing such services and must have necessary
> licensing and permissions in the locations where you make them available."

The argument, in short: this app performs none of those three things. It does not
trade, invest, or manage anyone's money. It records sales a business has already made,
the way accounting software does, and there is no path through the app by which money
moves. The comparable Indian apps — Vyapar, myBillBook, Zoho Invoice — are all
published under **Business**, not as financial institutions.

Make that case in the Resolution Center, factually and once. Do not resubmit the same
binary with the same notes hoping for a different reviewer; repeated resubmission
without change is what escalates to a 5.6 review suspension.

**The reliable fix is still the organization account.** Once the company is
incorporated and enrolled with its D-U-N-S number, the question stops being arguable.
The appeal above is worth making in parallel because it is free and may unblock you
sooner — not instead of enrolment.
