export type ServiceOffer = {
  name: string;
  price: string;
  priceUnits: string;
  input: string;
};

export const CANONICAL_SERVICES: ServiceOffer[] = [
  { name: 'web_brief', price: '0.15', priceUnits: '150000', input: 'public https URL' },
  { name: 'ai_synthesis', price: '0.25', priceUnits: '250000', input: 'prompt text' },
  { name: 'chain_audit', price: '0.50', priceUnits: '500000', input: '0x address' },
];

export function toolName(service: string) {
  return `nexus_${service}`;
}

export async function loadCatalog(workerUrl: string, fetchImpl: typeof fetch = fetch): Promise<ServiceOffer[]> {
  try {
    const response = await fetchImpl(`${workerUrl.replace(/\/$/, '')}/.well-known/nexus402.json`);
    if (!response.ok) return CANONICAL_SERVICES;
    const body = (await response.json()) as { services?: Array<Record<string, string | number>> };
    const services = (body.services ?? [])
      .map((service) => ({
        name: String(service.name ?? ''),
        price: String(service.price ?? ''),
        priceUnits: String(service.priceUnits ?? ''),
        input: String(service.input ?? 'prompt text'),
      }))
      .filter((service) => service.name && service.price);
    return services.length > 0 ? services : CANONICAL_SERVICES;
  } catch {
    return CANONICAL_SERVICES;
  }
}
