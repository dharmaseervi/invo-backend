# App Store review notes

> **This is a new app record: bundle `com.invobilling.app`, version 1.0.**
>
> It is not an update to the ebillz record (Apple ID 6740260986). ebillz 1.0.3 shipped
> as a universal app, and Apple rejects any update that drops a device family — this app
> is iPhone-only. ebillz is removed from sale so there is no live duplicate, and the
> suspended Invo Billing record is deleted, which frees the name and clears the 5.6
> suspension from the account.
>
> Store copy and screenshots live in `~/Desktop/invo-billing-appstore/`.


Paste section 1 into **App Store Connect → your build → App Review Information →
Notes for Review**. It is written to satisfy Guideline 2.3.1, which requires features
to be "described with specificity" and warns that "generic descriptions will be
rejected".

Sections 2 and 3 are for you, not for Apple.

---

## 1. Notes for Review — paste this

> **What this app is**
>
> Invo Billing is GST billing and inventory software for small retailers and traders in India. It creates invoices that comply with Indian GST rules, tracks stock, and produces the summaries needed to file a GSTR-1 return. It is bookkeeping software, in the same category as Zoho Invoice, Vyapar or QuickBooks.
>
> **It does not handle, hold, transfer or process money.** There is no payment gateway, card entry, wallet, bank connection or in-app purchase. Where the app says "payment", the user is recording cash or a bank transfer that already happened outside the app, exactly as they would in a paper ledger.
>
> **Demo account**
> Email: dharmaseervijb18239+demo@gmail.com
> Password: DemoScreens#2026
>
> Email-verified and pre-loaded with a sample business, "Sharma Textiles & Traders": 8 clients, 12 stock items in 4 categories, 10 invoices (paid, part-paid, overdue, draft), 4 payments, 5 expenses and 2 estimates, so every screen has real data on first launch. Delhi clients are billed CGST+SGST and out-of-state clients IGST, so the GST report shows both. No further sign-up, OTP or verification is needed.
>
> **Every feature, and how to reach it**
>
> Five tabs: Home, Invoices, Clients, Items, More.
>
> 1. Home - revenue for the selected period, counts of invoices, clients and items, a seven-day revenue chart, and recent invoices.
>
> 2. Invoices - tap "+" to create one: choose a client, search and add items, set quantity, rate, per-line discount and GST rate. Save as a draft, then "Issue" to assign the invoice number and deduct stock. An issued invoice can be cancelled, exported as a PDF, or emailed to the client.
>
> 3. Clients - name, phone, email, address, state and GSTIN. Tap a client for their invoices, outstanding balance and ledger.
>
> 4. Items - SKU, HSN code, unit, cost price, selling price, GST rate, stock quantity and low-stock threshold. Tap an item to record stock received and see its movement history. Items can be found by scanning a barcode with the camera (permission requested on first use). Items can also be printed as shelf labels on a Brother QL-series printer over Wi-Fi or Bluetooth; this needs Local Network permission and the physical printer. A reviewer without that hardware will see the printer search find no devices. That is expected and is not a hidden feature.
>
> 5. More - Estimates (quotations, convertible to invoices); Credit notes (returns stock and reduces the GST owed); Payments (applied to that client's oldest unpaid invoices); Expenses; Ledger (running balance per client); Reports (stock on hand, receivables ageing, and GSTR-1 with CSV export); Company settings (name, GSTIN, address and bank details, printed on the invoice PDF); Invoice template (three PDF layouts); Biometric lock (optional, off by default); Account (sign out, or delete the account and all its data).
>
> **Permissions, and why**
>
> Camera - scanning a product barcode; optional, items can always be typed. Photo Library - reading a barcode from an existing photo; optional. Local Network - finding a Brother label printer; optional, used only on the label printing screen. Face ID - the optional app lock; off by default. Notifications - overdue invoice reminders; optional, and only requested if turned on in More.
>
> **There are no hidden, dormant or undocumented features.** The app contains no web views and loads no remote code or remote configuration. Development-only helpers are excluded from release builds at compile time and are not present in this binary.
>
> A web version of the same account is available at https://invobilling.com/app - the same demo credentials work there if that is easier to review.

---

## 2. Before you resubmit — check each of these

- [ ] **The demo password above is filled in, and you have signed in with it yourself**,
      on a device, today. A demo account that does not work is the single commonest
      cause of a 2.3.1 rejection.
- [ ] **Primary category is Business, and Finance is removed as secondary.** Zoho Invoice, Vyapar, myBillBook
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
