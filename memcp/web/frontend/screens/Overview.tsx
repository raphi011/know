
import React, { useEffect, useState } from 'react';
import { AreaChart, Area, XAxis, YAxis, ResponsiveContainer, Tooltip } from 'recharts';
import { Link } from 'react-router-dom';
import { backend } from '../backend';

const StatCard = ({ title, value, trend, trendIcon, icon, color }: any) => (
  <div className="bg-surface-dark rounded-xl p-5 border border-[#233f48] relative overflow-hidden group flex flex-col justify-between min-h-[140px]">
    <div className="absolute top-2 right-2 opacity-5 group-hover:opacity-10 transition-opacity pointer-events-none">
      <span className={`material-symbols-outlined text-7xl ${color}`}>{icon}</span>
    </div>
    <div className="relative z-10">
      <p className="text-text-secondary text-xs font-bold uppercase tracking-widest mb-1">{title}</p>
      <h3 className="text-white text-3xl font-black tracking-tight">{value}</h3>
    </div>
    <div className="flex items-center gap-2 mt-4 relative z-10">
      <span className={`${trendIcon === 'warning' ? 'bg-orange-500/10 text-orange-400' : 'bg-green-500/10 text-green-400'} text-[10px] px-2 py-0.5 rounded flex items-center gap-1 font-bold uppercase tracking-wider`}>
        <span className="material-symbols-outlined text-[14px]">{trendIcon === 'warning' ? 'warning' : 'trending_up'}</span> {trend}
      </span>
      <span className="text-text-secondary text-[10px] uppercase font-medium">vs week</span>
    </div>
  </div>
);

const Overview = () => {
  const [data, setData] = useState<any>(null);
  const [recentMemories, setRecentMemories] = useState<any[]>([]);

  useEffect(() => {
    const fetchData = async () => {
      const [overviewRes, recentRes] = await Promise.all([
        backend.getOverview(),
        backend.getRecentMemories()
      ]);
      setData(overviewRes);
      setRecentMemories(recentRes);
    };
    fetchData();
  }, []);

  if (!data) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center bg-background-dark text-text-secondary">
        <div className="size-10 border-2 border-primary border-t-transparent rounded-full animate-spin mb-4"></div>
        <p className="text-xs font-black uppercase tracking-[0.2em]">Synchronizing Nodes...</p>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full animate-page-in overflow-hidden">
      <header className="flex items-center justify-between px-6 py-4 border-b border-[#233f48] bg-background-dark/80 backdrop-blur-md sticky top-0 z-30">
        <div className="flex flex-col">
          <h2 className="text-white text-xl font-black leading-none uppercase tracking-tight">Overview</h2>
          <p className="text-text-secondary text-xs mt-1 font-medium">System Status: Optimal</p>
        </div>
        <div className="flex items-center gap-4">
          <div className="hidden md:flex items-center relative w-64 lg:w-80">
            <span className="material-symbols-outlined absolute left-3 text-text-secondary text-[20px]">search</span>
            <input className="w-full h-10 bg-surface-dark border border-[#233f48] rounded-lg pl-10 pr-4 text-sm text-white placeholder-text-secondary focus:ring-1 focus:ring-primary focus:border-primary transition-all" placeholder="Quick search..." type="text"/>
          </div>
          <div className="h-9 w-9 rounded-lg bg-gradient-to-br from-primary to-purple-600 p-[1.5px] cursor-pointer shadow-lg shadow-primary/10">
            <div className="h-full w-full rounded-[7px] bg-background-dark flex items-center justify-center">
              <span className="text-xs font-black text-white uppercase">AA</span>
            </div>
          </div>
        </div>
      </header>
      
      <div className="flex-1 overflow-y-auto p-6 md:p-8">
        <div className="max-w-[1400px] mx-auto flex flex-col gap-8">
          {/* Top Stats Grid */}
          <div className="grid grid-cols-1 md:grid-cols-3 gap-5">
            {data.stats.map((stat: any, idx: number) => (
              <StatCard key={idx} {...stat} />
            ))}
          </div>

          {/* Main Content Grid */}
          <div className="grid grid-cols-1 xl:grid-cols-3 gap-8">
            {/* Chart Area */}
            <div className="xl:col-span-2 bg-surface-dark rounded-xl border border-[#233f48] p-6 flex flex-col shadow-sm">
              <div className="flex items-start justify-between mb-8">
                <div>
                  <h3 className="text-white text-lg font-bold">Memory Velocity</h3>
                  <p className="text-text-secondary text-sm">Read/Write operations over last 24h</p>
                </div>
                <div className="text-right">
                  <p className="text-3xl font-black text-white tracking-tighter">3,402</p>
                  <p className="text-[10px] text-primary font-bold uppercase tracking-widest">Ops / Sec</p>
                </div>
              </div>
              <div className="flex-1 w-full min-h-[280px]">
                <ResponsiveContainer width="100%" height="100%">
                  <AreaChart data={data.velocityData}>
                    <defs>
                      <linearGradient id="colorVal" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="5%" stopColor="#13b6ec" stopOpacity={0.3}/>
                        <stop offset="95%" stopColor="#13b6ec" stopOpacity={0}/>
                      </linearGradient>
                    </defs>
                    <Tooltip 
                      contentStyle={{ backgroundColor: '#1a2c35', border: '1px solid #233f48', borderRadius: '8px' }}
                      itemStyle={{ color: '#13b6ec', fontWeight: 'bold' }}
                    />
                    <Area type="monotone" dataKey="val" stroke="#13b6ec" strokeWidth={3} fillOpacity={1} fill="url(#colorVal)" />
                    <XAxis dataKey="name" stroke="#587a8a" fontSize={10} axisLine={false} tickLine={false} />
                    <YAxis hide={true} />
                  </AreaChart>
                </ResponsiveContainer>
              </div>
            </div>

            {/* Distribution Column */}
            <div className="xl:col-span-1 bg-surface-dark rounded-xl border border-[#233f48] p-6 flex flex-col shadow-sm min-h-[400px]">
              <h3 className="text-white text-lg font-bold mb-1">Entity Distribution</h3>
              <p className="text-text-secondary text-sm mb-8">Classification by knowledge type</p>
              <div className="space-y-6 flex-1">
                {data.distribution.map((item: any) => (
                  <div key={item.label}>
                    <div className="flex justify-between text-xs mb-2">
                      <span className="text-white font-bold uppercase tracking-wider">{item.label}</span>
                      <span className="text-primary font-black">{item.val}%</span>
                    </div>
                    <div className="w-full bg-[#101d22] rounded-full h-2">
                      <div className={`${item.color} h-full rounded-full transition-all duration-1000`} style={{ width: `${item.val}%` }}></div>
                    </div>
                  </div>
                ))}
              </div>
              <div className="mt-8 p-3.5 bg-[#101d22] rounded-xl border border-[#233f48] flex items-start gap-3 shadow-inner">
                <span className="material-symbols-outlined text-primary text-[18px] shrink-0 mt-0.5">info</span>
                <p className="text-[11px] text-text-secondary leading-relaxed font-medium italic">Clustering algorithm automatically balances these every 12 hours based on semantic density.</p>
              </div>
            </div>
          </div>

          {/* Recent Memories Section - Full Width List */}
          <div className="bg-surface-dark rounded-2xl border border-[#233f48] overflow-hidden flex flex-col shadow-xl mb-12">
            <div className="px-6 py-5 border-b border-[#233f48] flex justify-between items-center bg-black/10">
              <div className="flex items-center gap-3">
                <span className="material-symbols-outlined text-primary">history</span>
                <h3 className="text-white text-lg font-bold">Live Memory Stream</h3>
              </div>
              <Link to="/search" className="text-xs font-black text-primary uppercase tracking-widest hover:underline decoration-primary/30">View All Storage</Link>
            </div>
            <div className="divide-y divide-[#233f48]">
              {recentMemories.map((mem) => (
                <Link key={mem.id} to={`/entity/${mem.id}`} className="flex items-center gap-6 p-5 hover:bg-white/5 transition-all group">
                  <div className="size-10 rounded-xl bg-primary/5 border border-primary/20 flex items-center justify-center text-primary group-hover:scale-110 transition-transform">
                    <span className="material-symbols-outlined text-xl">{mem.icon}</span>
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-3 mb-1">
                      <span className="text-[10px] font-black text-text-secondary uppercase tracking-widest px-1.5 py-0.5 bg-[#101d22] border border-white/5 rounded">{mem.type}</span>
                      <span className="text-[10px] font-mono text-text-secondary opacity-40 uppercase">{mem.id}</span>
                    </div>
                    <p className="text-slate-300 text-sm font-medium truncate group-hover:text-white transition-colors">{mem.content}</p>
                  </div>
                  <div className="text-right shrink-0">
                    <p className="text-[10px] font-black text-text-secondary uppercase tracking-[0.1em]">{mem.time}</p>
                    <span className="material-symbols-outlined text-text-secondary text-sm opacity-0 group-hover:opacity-100 group-hover:translate-x-1 transition-all">chevron_right</span>
                  </div>
                </Link>
              ))}
            </div>
            {recentMemories.length === 0 && (
              <div className="p-12 text-center">
                <p className="text-text-secondary italic text-sm">No recent activity detected in the last cycle.</p>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};

export default Overview;
