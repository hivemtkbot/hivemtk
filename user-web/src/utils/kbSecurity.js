export const KB_SECURITY_POLICY = {
  crossTenantAccess: 'forbidden',
  publicAccess: 'allowed_with_warning',
  playgroundAutoFilter: true,
  openApiTokenBinding: 'required',
  requireSource: true
};

export const checkKbAccess = (kb, currentMerchantId) => {
  if (!kb) return { allowed: false, reason: 'not_found' }
  if (kb.is_public || kb.owner_id === 0) {
    return { allowed: true, scope: 'public' }
  }
  if (kb.merchant_id === currentMerchantId || kb.owner_id === currentMerchantId) {
    return { allowed: true, scope: 'tenant' }
  }
  return { allowed: false, reason: 'cross_tenant' }
};

export const buildPlaygroundFilter = (currentMerchantId) => ({
  merchant_id: currentMerchantId,
  include_public: true
});
