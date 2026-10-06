"use client";

import { useRouter } from "next/navigation";
import { startTransition } from "react";

import { Button } from "@/components/ui/button";

// The profile read failed: the database is unreachable, DATABASE_URL_APP is
// unset, or the page itself broke. The cause is in the server log, not here.
//
// reset() alone re-renders the boundary's children from the client cache; it
// does not re-run the failed server read. Refreshing the route first does.
export default function ProfileError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const router = useRouter();
  void error;

  return (
    <div className="mx-auto w-full max-w-content space-y-4 px-4 pt-2 pb-8 sm:px-6 lg:px-8">
      <header className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">Profile</h1>
        <p className="text-sm text-content-muted">
          The profile could not be loaded. Nothing you saved is affected.
        </p>
      </header>
      <Button
        variant="outline"
        onClick={() =>
          startTransition(() => {
            router.refresh();
            reset();
          })
        }
      >
        Try again
      </Button>
    </div>
  );
}
