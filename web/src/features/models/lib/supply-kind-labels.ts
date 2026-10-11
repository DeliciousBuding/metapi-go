// metapi-go/features/models — supply-kind i18n label keys.
//
// Single owner for the labels of the three supply kinds; the detail sheet's
// supply section and the list page's sourceKind facet options both read it,
// so filter and detail never drift apart.
export const supplyKindLabels = {
  direct: 'models.supply.direct',
  account: 'models.supply.account',
  oauth: 'models.supply.oauth',
} as const
