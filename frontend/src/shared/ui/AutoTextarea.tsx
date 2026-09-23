import { useLayoutEffect, useRef, type TextareaHTMLAttributes } from "react";
// Expand saved text at the current viewport width, including when a hidden tab opens.
export function AutoTextarea(
  props: TextareaHTMLAttributes<HTMLTextAreaElement>,
) {
  const ref = useRef<HTMLTextAreaElement>(null);
  useLayoutEffect(() => {
    const element = ref.current;
    if (!element) return;
    let width = -1;
    const fit = () => {
      if (!element.clientWidth) return;
      element.style.height = "auto";
      element.style.height = element.scrollHeight + 2 + "px";
    };
    fit();
    const observer = new ResizeObserver(() => {
      if (element.clientWidth !== width) {
        width = element.clientWidth;
        fit();
      }
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, [props.value]);
  return <textarea {...props} ref={ref} />;
}
