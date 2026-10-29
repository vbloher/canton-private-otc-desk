import { createContext, use, type ReactNode } from "react";
import type { Services } from "@/application/ports";

const ServicesContext = createContext<Services | null>(null);

export function ServicesProvider({ services, children }: { services: Services; children: ReactNode }) {
  return <ServicesContext value={services}>{children}</ServicesContext>;
}

export function useServices(): Services {
  const services = use(ServicesContext);
  if (services === null) throw new Error("useServices must be used inside <ServicesProvider>");
  return services;
}
