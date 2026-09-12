/**
 * GST state list — 28 states + 8 union territories, in the order GSTIN state codes are
 * assigned. Kept identical to the iOS list (Core/IndianStates.swift) on purpose: the
 * place of supply is compared against the company's state as a string, so a client
 * created on the web and one created on the phone have to spell it the same way or the
 * same supply would be taxed IGST on one and CGST+SGST on the other.
 */
export const INDIAN_STATES = [
  "Jammu and Kashmir", "Himachal Pradesh", "Punjab", "Chandigarh", "Uttarakhand",
  "Haryana", "Delhi", "Rajasthan", "Uttar Pradesh", "Bihar",
  "Sikkim", "Arunachal Pradesh", "Nagaland", "Manipur", "Mizoram",
  "Tripura", "Meghalaya", "Assam", "West Bengal", "Jharkhand",
  "Odisha", "Chhattisgarh", "Madhya Pradesh", "Gujarat",
  "Daman and Diu", "Dadra and Nagar Haveli", "Maharashtra", "Andhra Pradesh",
  "Karnataka", "Goa", "Lakshadweep", "Kerala", "Tamil Nadu",
  "Puducherry", "Andaman and Nicobar Islands", "Telangana", "Ladakh",
  "Other Territory",
] as const;
