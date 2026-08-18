import { Button } from "@/components/ui/button"

export default function Home() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-4">
      <h1 className="text-2xl font-semibold">Welcome to Next.js</h1>
      <Button>Get started</Button>
    </div>
  )
}
