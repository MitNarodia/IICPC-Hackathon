import React from 'react';
import { NavLink } from 'react-router-dom';
import { 
  LayoutDashboard, 
  Send, 
  Box, 
  Activity, 
  Network, 
  Trophy, 
  BookOpen, 
  Cpu
} from 'lucide-react';
import { cn } from '@/utils/cn';

const navigation = [
  { name: 'Overview', href: '/', icon: LayoutDashboard },
  { name: 'Dashboard', href: '/dashboard', icon: Activity },
  { name: 'Submissions', href: '/submissions', icon: Send },
  { name: 'Sandbox Explorer', href: '/sandbox', icon: Box },
  { name: 'Bot Fleet', href: '/fleet', icon: Network },
  { name: 'Telemetry', href: '/telemetry', icon: Cpu },
  { name: 'Leaderboard', href: '/leaderboard', icon: Trophy },
  { name: 'Architecture', href: '/architecture', icon: Network },
  { name: 'Documentation', href: '/docs', icon: BookOpen },
];

export function Sidebar() {
  return (
    <div className="flex h-full w-64 flex-col bg-card border-r border-gray-800">
      <div className="flex h-16 shrink-0 items-center px-6 border-b border-gray-800">
        <Cpu className="h-6 w-6 text-primary mr-2" />
        <span className="text-lg font-bold text-gray-100 tracking-tight">IICPC Platform</span>
      </div>
      <div className="flex flex-1 flex-col overflow-y-auto px-4 py-4">
        <nav className="flex-1 space-y-1">
          {navigation.map((item) => (
            <NavLink
              key={item.name}
              to={item.href}
              className={({ isActive }) =>
                cn(
                  isActive ? 'bg-primary/10 text-primary' : 'text-gray-400 hover:bg-gray-800 hover:text-gray-100',
                  'group flex items-center rounded-md px-3 py-2 text-sm font-medium transition-colors'
                )
              }
            >
              <item.icon
                className={cn('mr-3 h-5 w-5 flex-shrink-0')}
                aria-hidden="true"
              />
              {item.name}
            </NavLink>
          ))}
        </nav>
      </div>
    </div>
  );
}
