export function isBalanceLow(
  balance: number | null | undefined,
  threshold: number | null | undefined,
) {
  return balance != null
    && Number.isFinite(balance)
    && threshold != null
    && Number.isFinite(threshold)
    && threshold > 0
    && balance < threshold
}
