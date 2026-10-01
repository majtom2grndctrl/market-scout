"use client";

import { Button } from "@/components/ui/button";

// The profile read failed: the database is down, or DATABASE_URL_APP is unset.
// Nothing below can be shown honestly, so the page says so and offers a retry.
export default function ProfileError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  void error;

  return (
    <div className="mx-auto w-full max-w-content space-y-4 px-4 py-8 sm:px-6 lg:px-8">
      <header className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">Profile</h1>
        <p className="text-sm text-content-muted">
          The profile could not be loaded. Nothing you saved is lost; the database did not answer.
        </p>
      </header>
      <Button variant="outline" onClick={reset}>
        Try again
      </Button>
    </div>
  );
}
