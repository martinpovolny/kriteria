import { useEffect, useState } from "react";
import { apiGet, type Me } from "@/lib/api";

// Shows the currently logged-in teacher/director's name in the top bar.
export default function UserBadge() {
  const [me, setMe] = useState<Me | null>(null);

  useEffect(() => {
    apiGet<Me>("/api/me").then(setMe).catch(() => setMe(null));
  }, []);

  if (!me) return null;

  return (
    <span className="text-sm text-muted-foreground" title={me.email}>
      {me.display_name}
    </span>
  );
}
