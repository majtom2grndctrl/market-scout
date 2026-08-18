"use client";

import { Button } from "@/components/ui/button";

// Catches failures the status page itself cannot report as a fact: the read
// model — the database this page reads for every figure below — is down.
export default function StatusError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  void error;

  return (
    <div className="mx-auto w-full max-w-wide space-y-4 px-4 py-8 sm:px-6 lg:px-8">
      <header className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">Status</h1>
        <p className="text-sm text-muted-foreground">
          The read model is unreachable. No figures below could be collected.
        </p>
      </header>
      <Button variant="outline" onClick={reset}>
        Try again
      </Button>
    </div>
  );
}
