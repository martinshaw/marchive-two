import { useEffect, useRef, useState } from "react";

export function CellTip() {
  const [tip, setTip] = useState(null);
  const ref = useRef(null);
  useEffect(() => {
    let timer = 0;
    function hide() {
      window.clearTimeout(timer);
      setTip(null);
    }
    function over(event) {
      const cell = event.target.closest?.("table.grid tbody td");
      window.clearTimeout(timer);
      if (!cell) { setTip(null); return; }
      const x = event.clientX;
      const y = event.clientY;
      timer = window.setTimeout(() => {
        const nodes = [cell, ...cell.querySelectorAll(".primary, .sub, a, .fault, .pill")];
        const overflow = nodes.some((node) => node.scrollWidth > node.clientWidth + 1 || node.scrollHeight > node.clientHeight + 1);
        const text = cell.innerText.replace(/\s+\n/g, "\n").trim();
        if (!overflow || !text) { setTip(null); return; }
        setTip({ text, x, y });
      }, 350);
    }
    function out(event) {
      if (event.target.closest?.("table.grid tbody td")) hide();
    }
    document.addEventListener("mouseover", over);
    document.addEventListener("mouseout", out);
    document.addEventListener("scroll", hide, true);
    return () => {
      window.clearTimeout(timer);
      document.removeEventListener("mouseover", over);
      document.removeEventListener("mouseout", out);
      document.removeEventListener("scroll", hide, true);
    };
  }, []);
  useEffect(() => {
    const node = ref.current;
    if (!node || !tip) return;
    const box = node.getBoundingClientRect();
    let left = tip.x + 12;
    let top = tip.y + 16;
    if (left + box.width > window.innerWidth - 8) left = window.innerWidth - box.width - 8;
    if (top + box.height > window.innerHeight - 8) top = tip.y - box.height - 10;
    node.style.left = `${Math.max(8, left)}px`;
    node.style.top = `${Math.max(8, top)}px`;
  }, [tip]);
  return <div ref={ref} className="cell-tip" hidden={!tip} style={tip ? { left: tip.x + 12, top: tip.y + 16 } : undefined}>{tip?.text}</div>;
}
