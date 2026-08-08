import * as React from "react";

interface CitationNumberContextValue {
  nextIndex: () => number;
}

const CitationNumberContext = React.createContext<CitationNumberContextValue>({
  nextIndex: () => 0
});

export function CitationNumberProvider({ children }: { children: React.ReactNode }) {
  const counter = React.useRef(0);
  const value: CitationNumberContextValue = React.useMemo(
    () => ({
      nextIndex: () => {
        counter.current += 1;
        return counter.current;
      }
    }),
    [counter]
  );
  return <CitationNumberContext.Provider value={value}>{children}</CitationNumberContext.Provider>;
}

export function useCitationNumber() {
  return React.useContext(CitationNumberContext);
}
