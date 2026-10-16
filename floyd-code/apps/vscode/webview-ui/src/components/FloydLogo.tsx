import { useExtensionImageUrl } from "./hooks/useExtensionImageUrl";

export function FloydLogo({ className }: { className?: string }) {
  const logoUrl = useExtensionImageUrl("floyd-logo.png");

  if (!logoUrl) {
    return null;
  }

  return <img src={logoUrl} alt="FLOYD" className={className} aria-label="FLOYD" />;
}
