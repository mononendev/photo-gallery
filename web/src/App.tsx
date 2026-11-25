import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import Layout from './components/Layout';
import Events from './pages/Events';
import EventPage from './pages/EventPage';
import PhotoPage from './pages/PhotoPage';
import Search from './pages/Search';
import About from './pages/About';
import AdminLayout from './pages/admin/AdminLayout';
import AdminEvents from './pages/admin/AdminEvents';
import AdminEvent from './pages/admin/AdminEvent';

const queryClient = new QueryClient({ defaultOptions: { queries: { refetchOnWindowFocus: false } } });

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route element={<Layout />}>
            <Route path="/" element={<Events />} />
            <Route path="/events/:slug" element={<EventPage />} />
            <Route path="/events/:slug/photos/:id" element={<PhotoPage />} />
            <Route path="/search" element={<Search />} />
            <Route path="/about" element={<About />} />
          </Route>
          <Route path="/admin" element={<AdminLayout />}>
            <Route index element={<AdminEvents />} />
            <Route path="events/:slug" element={<AdminEvent />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
