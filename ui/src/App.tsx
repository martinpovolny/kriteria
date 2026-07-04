import { Routes, Route, Link } from 'react-router-dom'
import TeacherView from '@/pages/TeacherView'
import DirectorView from '@/pages/DirectorView'
import ParentView from '@/pages/ParentView'
import ErrorBoundary from '@/components/ErrorBoundary'

export default function App() {
  return (
    <ErrorBoundary>
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/ucitel" element={<TeacherView />} />
        <Route path="/reditel" element={<DirectorView />} />
        <Route path="/z/:slug" element={<ParentView />} />
        <Route path="*" element={<NotFound />} />
      </Routes>
    </ErrorBoundary>
  )
}

function Home() {
  return (
    <div className="min-h-full bg-background text-foreground">
      <header className="border-b border-border px-6 py-4">
        <h1 className="text-2xl font-bold">Kriteria</h1>
        <p className="text-muted-foreground text-sm">
          Kritériové hodnocení žáků
        </p>
      </header>
      <main className="mx-auto max-w-3xl px-6 py-12">
        <div className="space-y-6">
          <div className="rounded-lg border border-border p-6">
            <h2 className="text-lg font-semibold mb-2">Vítejte</h2>
            <p className="text-muted-foreground text-sm leading-relaxed">
              Tato aplikace slouží učitelům k zadávání kritériového hodnocení
              žáků. Rodičům umožňuje anonymní přístup k výsledkům jejich dítěte
              prostřednictvím dvojice vygenerovaných slov.
            </p>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Link
              to="/ucitel"
              className="rounded-lg border border-border p-6 hover:bg-muted transition-colors"
            >
              <h3 className="font-semibold mb-1">Pro učitele</h3>
              <p className="text-muted-foreground text-sm">
                Zadávání a přehled hodnocení žáků
              </p>
            </Link>
            <Link
              to="/reditel"
              className="rounded-lg border border-border p-6 hover:bg-muted transition-colors"
            >
              <h3 className="font-semibold mb-1">Pro ředitele</h3>
              <p className="text-muted-foreground text-sm">
                Přehled žáků a záznamy hodnocení
              </p>
            </Link>
          </div>
        </div>
      </main>
    </div>
  )
}

function NotFound() {
  return (
    <div className="min-h-full flex items-center justify-center">
      <p className="text-muted-foreground">Stránka nenalezena</p>
    </div>
  )
}
