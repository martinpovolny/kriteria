import { Component, type ReactNode } from "react";

interface Props {
  children: ReactNode;
}
interface State {
  hasError: boolean;
  error?: Error;
}

export default class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false };

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, info: any) {
    console.error("React error:", error, info);
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="min-h-full flex items-center justify-center p-8">
          <div className="max-w-md">
            <h2 className="text-lg font-semibold mb-2">Něco se pokazilo</h2>
            <p className="text-sm text-muted-foreground mb-4">
              {this.state.error?.message || "Neočekávaná chyba"}
            </p>
            <button
              onClick={() => this.setState({ hasError: false, error: undefined })}
              className="rounded-md bg-primary px-4 py-2 text-sm text-primary-foreground"
            >
              Zkusit znovu
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}
