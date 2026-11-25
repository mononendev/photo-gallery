import { Link, NavLink, Outlet, useLocation } from 'react-router-dom';
import DisclaimerModal from './DisclaimerModal';

const nav = [
  { to: '/', label: 'Events', end: true },
  { to: '/search', label: 'Search', end: false },
  { to: '/about', label: 'About', end: false },
];

export default function Layout() {
  const { pathname } = useLocation();
  return (
    <div className="min-h-screen flex flex-col bg-gray-950 text-gray-100">
      <header className="border-b border-gray-800 bg-gray-900/80 backdrop-blur sticky top-0 z-40">
        <div className="mx-auto max-w-7xl px-4 py-3 flex items-center gap-6">
          <Link to="/" className="font-semibold">Photo Gallery</Link>
          <nav className="flex gap-4 text-sm">
            {nav.map((n) => (
              <NavLink key={n.to} to={n.to} end={n.end}
                className={({ isActive }) => (isActive ? 'text-white' : 'text-gray-400 hover:text-gray-200')}>
                {n.label}
              </NavLink>
            ))}
          </nav>
        </div>
      </header>
      <main className="flex-1 mx-auto w-full max-w-7xl px-4 py-6">
        <Outlet />
      </main>
      {pathname.startsWith('/events/') && <DisclaimerModal />}
    </div>
  );
}
