import 'package:flutter/material.dart';

import 'student_letter_dialog.dart';
import 'student_letter_store.dart';

/// Mounted after initial identity setup, keyed to the root's authority window.
class StudentLetterGate extends StatefulWidget {
  const StudentLetterGate({
    super.key,
    required this.accountId,
    required this.store,
    required this.onReady,
    required this.child,
  });

  final String accountId;
  final StudentLetterStore store;
  final VoidCallback onReady;
  final Widget child;

  @override
  State<StudentLetterGate> createState() => _StudentLetterGateState();
}

class _StudentLetterGateState extends State<StudentLetterGate> {
  bool _checked = false, _ready = false, _queued = false;
  DialogRoute<void>? _dialog;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    var read = false;
    try {
      read = await widget.store.hasRead(widget.accountId);
    } catch (_) {
      // A missing/unreadable receipt leaves this letter unread.
    }
    if (!mounted) return;
    setState(() {
      _checked = true;
      _ready = read;
    });
    if (read) {
      widget.onReady();
    } else {
      _schedule();
    }
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _schedule();
  }

  void _schedule() {
    if (!_checked || _ready || _queued || _dialog != null) return;
    _queued = true;
    WidgetsBinding.instance.addPostFrameCallback((_) async {
      _queued = false;
      if (!mounted ||
          _ready ||
          _dialog != null ||
          ModalRoute.isCurrentOf(context) != true) {
        return;
      }
      final account = widget.accountId;
      var acknowledged = false;
      final route = DialogRoute<void>(
        context: context,
        barrierDismissible: false,
        builder: (_) => StudentLetterDialog(
          onRead: () async {
            await widget.store.markRead(account);
            acknowledged = true;
          },
        ),
      );
      _dialog = route;
      await Navigator.of(context).push(route);
      if (_dialog == route) _dialog = null;
      if (!mounted) return;
      if (!acknowledged) {
        _schedule();
        return;
      }
      setState(() => _ready = true);
      widget.onReady();
    });
    WidgetsBinding.instance.ensureVisualUpdate();
  }

  @override
  void dispose() {
    final route = _dialog;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (route?.isActive == true) route!.navigator?.removeRoute(route);
    });
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => ExcludeFocus(
    excluding: !_ready,
    child: AbsorbPointer(absorbing: !_ready, child: widget.child),
  );
}
