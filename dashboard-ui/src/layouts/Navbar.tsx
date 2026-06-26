import React from 'react';
import { Search, Bell, Settings } from 'lucide-react';
import { Button } from '@/components/ui/Button';

export function Navbar() {
  return (
    <header className="sticky top-0 z-40 flex h-16 shrink-0 items-center gap-x-4 border-b border-gray-800 bg-background/80 backdrop-blur-md px-4 shadow-sm sm:gap-x-6 sm:px-6 lg:px-8">
      <div className="flex flex-1 gap-x-4 self-stretch lg:gap-x-6">
        <form className="relative flex flex-1" action="#" method="GET">
          <label htmlFor="search-field" className="sr-only">
            Search
          </label>
          <Search
            className="pointer-events-none absolute inset-y-0 left-0 h-full w-5 text-gray-500"
            aria-hidden="true"
          />
          <input
            id="search-field"
            className="block h-full w-full border-0 py-0 pl-8 pr-0 text-gray-200 bg-transparent placeholder:text-gray-500 focus:ring-0 sm:text-sm"
            placeholder="Press Cmd+K to search..."
            type="search"
            name="search"
          />
        </form>
        <div className="flex items-center gap-x-4 lg:gap-x-6">
          <Button variant="ghost" size="sm" className="w-9 px-0">
            <Bell className="h-5 w-5" aria-hidden="true" />
            <span className="sr-only">View notifications</span>
          </Button>
          <div className="hidden lg:block lg:h-6 lg:w-px lg:bg-gray-800" aria-hidden="true" />
          <Button variant="ghost" size="sm" className="w-9 px-0">
            <Settings className="h-5 w-5" aria-hidden="true" />
            <span className="sr-only">Settings</span>
          </Button>
        </div>
      </div>
    </header>
  );
}
