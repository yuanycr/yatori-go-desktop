import { HashRouter, Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import Accounts from './pages/Accounts'
import Tasks from './pages/Tasks'
import Courses from './pages/Courses'
import Logs from './pages/Logs'
import Settings from './pages/Settings'
import About from './pages/About'

export default function App() {
  return (
    <HashRouter>
      <Routes>
        <Route path="/" element={<Layout />}>
          <Route index element={<Dashboard />} />
          <Route path="accounts" element={<Accounts />} />
          <Route path="tasks" element={<Tasks />} />
          <Route path="courses" element={<Courses />} />
          <Route path="logs" element={<Logs />} />
          <Route path="settings" element={<Settings />} />
          <Route path="about" element={<About />} />
        </Route>
      </Routes>
    </HashRouter>
  )
}
