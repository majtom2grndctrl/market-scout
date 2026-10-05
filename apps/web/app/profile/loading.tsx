import { Skeleton } from "@/components/ui/skeleton";

export default function ProfileLoading() {
  return (
    <div
      className="mx-auto w-full max-w-content space-y-10 px-4 py-8 sm:px-6 lg:px-8"
      aria-busy="true"
      aria-label="Loading profile"
    >
      <header className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">Profile</h1>
        <Skeleton className="h-4 w-72" />
      </header>
      {[0, 1, 2].map((i) => (
        <div key={i} className="space-y-3">
          <Skeleton className="h-3 w-24" />
          <Skeleton className="h-28 w-full rounded-lg" />
        </div>
      ))}
    </div>
  );
}
